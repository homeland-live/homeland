package script_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/homeland-live/homeland/internal/script"
)

func TestLoadAllSetsDeviceIDForBothIDForms(t *testing.T) {
	dir := t.TempDir()
	// A single device may be scripted under either identity; the loader has no
	// z2m/device context, so it loads both files and keys each by its own id.
	writeScript(t, dir, "living-room-lamp.js", `function handle(e, c) {}`)
	writeScript(t, dir, "0x00124b00247f9ca8.js", `function handle(e, c) {}`)
	// Non-".js" entries are skipped.
	writeScript(t, dir, "README.md", `ignore me`)

	scripts, err := script.LoadAll(dir)
	require.NoError(t, err)
	require.Len(t, scripts, 2)

	byID := make(map[string]*script.Script, len(scripts))
	for _, s := range scripts {
		byID[s.DeviceID] = s
	}
	_, hasFriendly := byID["living-room-lamp"]
	_, hasAddress := byID["0x00124b00247f9ca8"]
	assert.True(t, hasFriendly, "friendly-name script must be loaded and keyed by id")
	assert.True(t, hasAddress, "ieee-address script must be loaded and keyed by id")
}

func TestLoadSetsDeviceID(t *testing.T) {
	dir := t.TempDir()
	writeScript(t, dir, "0x00124b00247f9ca8.js", `function handle(e, c) {}`)

	s, err := script.Load(dir, "0x00124b00247f9ca8.js")
	require.NoError(t, err)
	assert.Equal(t, "0x00124b00247f9ca8.js", s.FileName)
	assert.Equal(t, "0x00124b00247f9ca8", s.DeviceID)
}

func TestLoadAllEmptyDirIsNotExist(t *testing.T) {
	_, err := script.LoadAll(t.TempDir())
	assert.ErrorIs(t, err, script.ErrNotExist)
}

func TestLoadAllSkipsUnreadableAndHiddenFiles(t *testing.T) {
	dir := t.TempDir()
	writeScript(t, dir, "lamp.js", `function handle(e, c) {}`)
	// An editor lockfile: a dangling symlink with a hidden name.
	require.NoError(t, os.Symlink("user@host.1234", filepath.Join(dir, ".#lamp.js")))
	// A broken symlink and a directory, both with a script name.
	require.NoError(t, os.Symlink("missing.js", filepath.Join(dir, "broken.js")))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "folder.js"), 0o700))
	// A symlink escaping the scripts dir is refused by os.Root.
	outside := filepath.Join(t.TempDir(), "outside.js")
	require.NoError(t, os.WriteFile(outside, []byte(`function handle(e, c) {}`), 0o600))
	require.NoError(t, os.Symlink(outside, filepath.Join(dir, "escape.js")))

	scripts, err := script.LoadAll(dir)
	require.NoError(t, err, "one bad file must not stop the others from loading")
	require.Len(t, scripts, 1)
	assert.Equal(t, "lamp", scripts[0].DeviceID)
}

func TestLoadAllOnlyBadFilesIsNotExist(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Symlink("missing.js", filepath.Join(dir, "broken.js")))

	_, err := script.LoadAll(dir)
	assert.ErrorIs(t, err, script.ErrNotExist)
}

func TestIsScriptFile(t *testing.T) {
	assert.True(t, script.IsScriptFile("lamp.js"))
	assert.True(t, script.IsScriptFile("0x00124b00247f9ca8.js"))
	assert.False(t, script.IsScriptFile(".#lamp.js"), "editor lockfiles are hidden")
	assert.False(t, script.IsScriptFile(".lamp.js.swp"))
	assert.False(t, script.IsScriptFile("lamp.js~"))
	assert.False(t, script.IsScriptFile("README.md"))
}
