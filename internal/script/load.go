package script

import (
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// ErrNotExist aliases fs.ErrNotExist. LoadAll and Load return errors matching
// it when there is nothing to load.
var ErrNotExist = fs.ErrNotExist

// IsScriptFile reports whether a file name names a device script: a ".js"
// file that is not hidden. Dotfiles (editor lock and swap files such as
// ".#lamp.js") are never scripts.
func IsScriptFile(name string) bool {
	return filepath.Ext(name) == FileExt && !strings.HasPrefix(name, ".")
}

// LoadAll reads every script file (see IsScriptFile) directly under dir,
// sorted by file name. Access is scoped to dir via os.Root. A file that cannot
// be read (a directory, a broken symlink or one escaping dir, no permission)
// is logged and skipped, so one bad file does not stop the others from
// loading. It returns an error matching ErrNotExist when dir is missing or
// holds no loadable scripts.
func LoadAll(dir string) ([]*Script, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := root.Close(); err != nil {
			slog.Error("script: failed to close scripts dir root", "err", err, "scriptsDir", dir)
		}
	}()

	// fs.ReadDir sorts entries by name.
	entries, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		return nil, err
	}

	scripts := make([]*Script, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !IsScriptFile(name) {
			continue
		}
		src, err := fs.ReadFile(root.FS(), name)
		if err != nil {
			slog.Warn("script: skipping unreadable script", "script", name, "scriptsDir", dir, "err", err)
			continue
		}
		scripts = append(scripts, NewScript(name, string(src)))
	}
	if len(scripts) == 0 {
		return nil, fmt.Errorf("script: no scripts in dir %q: %w", dir, ErrNotExist)
	}
	return scripts, nil
}

// Load reads the script file name (see IsScriptFile) under dir. Access is
// scoped to dir via os.Root, so name cannot escape it. A missing file returns
// an error matching ErrNotExist.
func Load(dir, name string) (*Script, error) {
	if !IsScriptFile(name) {
		return nil, fmt.Errorf("script: %q is not a script file", name)
	}

	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := root.Close(); err != nil {
			slog.Error("script: failed to close scripts dir root", "err", err, "scriptsDir", dir)
		}
	}()

	src, err := fs.ReadFile(root.FS(), name)
	if err != nil {
		return nil, err
	}
	return NewScript(name, string(src)), nil
}
