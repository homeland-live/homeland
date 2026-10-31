package scripts

import (
	"errors"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/homeland-live/homeland/internal/script"
)

// WatchDebounce is how long a script file must be quiet before it is synced.
// One editor save is often several events (truncate, write, rename over), so
// syncing on each would compile a half-written file and briefly drop or break
// the handler.
const WatchDebounce = 100 * time.Millisecond

// watcher keeps the handler cache in sync with the scripts directory. Events
// only mark a script file (see script.IsScriptFile) as changed. Once it has
// been quiet for the debounce interval, the file on disk decides: a readable
// file is recompiled and a missing one dropped, whatever the events were.
// Event handling and syncs run on one goroutine, so they never overlap.
type watcher struct {
	svc *Service
	dir string
	fsw *fsnotify.Watcher

	// debounce delays each changed file's sync. Only the loop goroutine
	// calls it.
	debounce *debouncer

	wg   sync.WaitGroup
	done chan struct{}
}

// newWatcher starts watching the scripts directory in a background goroutine.
// A missing directory is not an error: the returned watcher is idle.
func newWatcher(svc *Service) (*watcher, error) {
	dir := svc.cfg.Scripts.Dir

	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	done := make(chan struct{})
	w := &watcher{
		svc:      svc,
		dir:      dir,
		fsw:      fsw,
		debounce: newDebouncer(WatchDebounce, done),
		done:     done,
	}

	if err := fsw.Add(dir); err != nil {
		if cerr := fsw.Close(); cerr != nil {
			slog.Error("svc/scripts: failed to close watcher", "err", cerr)
		}
		if errors.Is(err, script.ErrNotExist) {
			slog.Warn("svc/scripts: scripts dir does not exist, watcher disabled", "scriptsDir", dir)
			w.fsw = nil
			return w, nil
		}
		return nil, err
	}

	slog.Info("svc/scripts: watching scripts dir", "scriptsDir", dir)
	w.wg.Go(w.loop)
	return w, nil
}

// loop consumes fsnotify events and due syncs until close is called.
func (w *watcher) loop() {
	for {
		select {
		case <-w.done:
			return
		case event, ok := <-w.fsw.Events:
			if !ok {
				return
			}
			w.handleEvent(event)
		case f := <-w.debounce.due:
			if w.debounce.take(f) {
				w.sync(f.name)
			}
		case err, ok := <-w.fsw.Errors:
			if !ok {
				return
			}
			slog.Error("svc/scripts: watcher error", "err", err)
		}
	}
}

// handleEvent (re)starts the debounce timer of the script file an event is
// about, superseding a sync already due but not yet run, so a file is synced
// only once its burst of events ends. Events for other files are ignored.
func (w *watcher) handleEvent(event fsnotify.Event) {
	name := filepath.Base(event.Name)
	if !script.IsScriptFile(name) {
		return
	}
	w.debounce.touch(name)
}

// sync makes the cache match the file name on disk: a readable file is
// recompiled, a missing one has its handler dropped. Any other read error (a
// directory, no permission) is logged and leaves the cache as is.
func (w *watcher) sync(name string) {
	devID := strings.TrimSuffix(name, script.FileExt)

	s, err := script.Load(w.dir, name)
	switch {
	case errors.Is(err, script.ErrNotExist):
		if w.svc.drop(devID) {
			slog.Info("svc/scripts: removed handler", "script", name)
		}
	case err != nil:
		slog.Error("svc/scripts: failed to load changed script", "script", name, "err", err)
	default:
		if err := w.svc.reload(s); err != nil {
			slog.Error("svc/scripts: failed to reload changed script", "script", name, "err", err)
			return
		}
		slog.Info("svc/scripts: reloaded handler", "script", name)
	}
}

// close stops the loop, cancels pending syncs and releases the fsnotify
// watcher.
func (w *watcher) close() {
	close(w.done)
	w.wg.Wait()
	w.debounce.stop()
	if w.fsw == nil {
		return
	}
	if err := w.fsw.Close(); err != nil {
		slog.Error("svc/scripts: failed to close watcher", "err", err)
	}
}
