package snippets_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/homeland-live/homeland/cmd/gen-docs/snippets"
)

// writeExamples lays out files under a temp dir and returns it as the baseDir
// Render resolves marker paths against.
func writeExamples(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	}
	return dir
}

func TestRenderFillsEmptyRegion(t *testing.T) {
	// An empty region is filled in place without touching the surrounding
	// sections or the next region.
	dir := writeExamples(t, map[string]string{
		"examples/a.js": "const a = 1;\n",
		"examples/b.js": "const b = 2;\n",
	})
	doc := "intro\n\n" +
		"<!-- gen-docs:begin examples/a.js -->\n" +
		"<!-- gen-docs:end -->\n\n" +
		"between\n\n" +
		"<!-- gen-docs:begin examples/b.js -->\n" +
		"<!-- gen-docs:end -->\n\n" +
		"outro\n"

	got, err := snippets.Render([]byte(doc), dir)
	require.NoError(t, err)

	want := "intro\n\n" +
		"<!-- gen-docs:begin examples/a.js -->\n```js\nconst a = 1;\n```\n<!-- gen-docs:end -->\n\n" +
		"between\n\n" +
		"<!-- gen-docs:begin examples/b.js -->\n```js\nconst b = 2;\n```\n<!-- gen-docs:end -->\n\n" +
		"outro\n"
	assert.Equal(t, want, string(got))
}

func TestRenderIsIdempotent(t *testing.T) {
	dir := writeExamples(t, map[string]string{"examples/a.js": "const a = 1;\n"})
	doc := "<!-- gen-docs:begin examples/a.js -->\n<!-- gen-docs:end -->\n"

	once, err := snippets.Render([]byte(doc), dir)
	require.NoError(t, err)
	twice, err := snippets.Render(once, dir)
	require.NoError(t, err)
	assert.Equal(t, string(once), string(twice), "rendering an already-rendered doc must not change it")
}

func TestRenderReplacesStaleBody(t *testing.T) {
	// A region whose body no longer matches the file is rewritten.
	dir := writeExamples(t, map[string]string{"examples/a.js": "fresh();\n"})
	doc := "<!-- gen-docs:begin examples/a.js -->\n```js\nstale();\n```\n<!-- gen-docs:end -->\n"

	got, err := snippets.Render([]byte(doc), dir)
	require.NoError(t, err)
	assert.Contains(t, string(got), "fresh();")
	assert.NotContains(t, string(got), "stale();")
}

func TestRenderTrimsAndPicksLanguage(t *testing.T) {
	dir := writeExamples(t, map[string]string{
		"examples/a.js":   "\n\nconst a = 1;\n\n\n", // surrounding blank lines trimmed
		"conf/thing.yaml": "key: value\n",
	})
	doc := "<!-- gen-docs:begin examples/a.js -->\n<!-- gen-docs:end -->\n" +
		"<!-- gen-docs:begin conf/thing.yaml -->\n<!-- gen-docs:end -->\n"

	got, err := snippets.Render([]byte(doc), dir)
	require.NoError(t, err)
	assert.Contains(t, string(got), "```js\nconst a = 1;\n```")
	assert.Contains(t, string(got), "```yaml\nkey: value\n```")
}

func TestRenderGlob(t *testing.T) {
	// A pattern renders every match in lexical order under a heading of its
	// file name, separated by rules, skipping hidden files, directories and
	// non-matches.
	dir := writeExamples(t, map[string]string{
		"examples/b.js":         "b();\n",
		"examples/a.js":         "a();\n",
		"examples/.hidden.js":   "hidden();\n",
		"examples/notes.txt":    "not js\n",
		"examples/sub.js/inner": "a directory named like a script\n",
		"other/outside-glob.js": "other();\n",
	})
	doc := "<!-- gen-docs:begin examples/*.js -->\nstale\n<!-- gen-docs:end -->\n"

	got, err := snippets.Render([]byte(doc), dir)
	require.NoError(t, err)

	want := "<!-- gen-docs:begin examples/*.js -->\n" +
		"### a.js\n\n```js\na();\n```\n\n---\n\n" +
		"### b.js\n\n```js\nb();\n```\n" +
		"<!-- gen-docs:end -->\n"
	assert.Equal(t, want, string(got))

	twice, err := snippets.Render(got, dir)
	require.NoError(t, err)
	assert.Equal(t, string(got), string(twice), "a rendered glob region must be stable")
}

func TestRenderDotSlashPrefix(t *testing.T) {
	// A leading ./ works the same for a plain path and a pattern.
	dir := writeExamples(t, map[string]string{"examples/a.js": "a();\n"})
	doc := "<!-- gen-docs:begin ./examples/a.js -->\n<!-- gen-docs:end -->\n" +
		"<!-- gen-docs:begin ./examples/*.js -->\n<!-- gen-docs:end -->\n"

	got, err := snippets.Render([]byte(doc), dir)
	require.NoError(t, err)
	want := "<!-- gen-docs:begin ./examples/a.js -->\n```js\na();\n```\n<!-- gen-docs:end -->\n" +
		"<!-- gen-docs:begin ./examples/*.js -->\n### a.js\n\n```js\na();\n```\n<!-- gen-docs:end -->\n"
	assert.Equal(t, want, string(got))
}

func TestRenderNoMarkersUnchanged(t *testing.T) {
	doc := "# Just a doc\n\nNo markers here.\n"
	got, err := snippets.Render([]byte(doc), t.TempDir())
	require.NoError(t, err)
	assert.Equal(t, doc, string(got))
}

func TestRenderPreservesTrailingNewline(t *testing.T) {
	dir := writeExamples(t, map[string]string{"examples/a.js": "a();\n"})
	region := "<!-- gen-docs:begin examples/a.js -->\n<!-- gen-docs:end -->"

	with, err := snippets.Render([]byte(region+"\n"), dir)
	require.NoError(t, err)
	assert.Equal(t, byte('\n'), with[len(with)-1], "a trailing newline must be kept")

	without, err := snippets.Render([]byte(region), dir)
	require.NoError(t, err)
	assert.NotEqual(t, byte('\n'), without[len(without)-1], "no trailing newline must not add one")
}

func TestRenderErrors(t *testing.T) {
	root := writeExamples(t, map[string]string{
		"docs/examples/a.js": "a();\n",
		"outside.js":         "outside();\n",
	})
	dir := filepath.Join(root, "docs")
	cases := map[string]struct{ doc, contains string }{
		"unknown file": {
			doc:      "<!-- gen-docs:begin examples/missing.js -->\n<!-- gen-docs:end -->\n",
			contains: "examples/missing.js",
		},
		"begin without end": {
			doc:      "<!-- gen-docs:begin examples/a.js -->\nno end here\n",
			contains: "no matching end",
		},
		"nested begin": {
			doc: "<!-- gen-docs:begin examples/a.js -->\n" +
				"<!-- gen-docs:begin examples/a.js -->\n<!-- gen-docs:end -->\n",
			contains: "nested",
		},
		"path escaping baseDir": {
			// outside.js exists, so only os.Root can reject it.
			doc:      "<!-- gen-docs:begin ../outside.js -->\n<!-- gen-docs:end -->\n",
			contains: "escapes",
		},
		"pattern escaping baseDir": {
			doc:      "<!-- gen-docs:begin ../*.js -->\n<!-- gen-docs:end -->\n",
			contains: "escapes",
		},
		"pattern matching nothing": {
			doc:      "<!-- gen-docs:begin examples/*.ts -->\n<!-- gen-docs:end -->\n",
			contains: "matches no files",
		},
		"malformed pattern": {
			doc:      "<!-- gen-docs:begin examples/[.js -->\n<!-- gen-docs:end -->\n",
			contains: "examples/[.js",
		},
		"stray end": {
			doc:      "text\n<!-- gen-docs:end -->\n",
			contains: "without a matching begin",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := snippets.Render([]byte(tc.doc), dir)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.contains)
		})
	}
}

func TestGenerateAndCheck(t *testing.T) {
	dir := writeExamples(t, map[string]string{"examples/a.js": "a();\n"})
	docPath := filepath.Join(dir, "README.md")
	require.NoError(t, os.WriteFile(docPath,
		[]byte("<!-- gen-docs:begin examples/a.js -->\n<!-- gen-docs:end -->\n"), 0o600))

	// Check reports a stale doc and writes nothing.
	stale, err := snippets.Check(docPath)
	require.NoError(t, err)
	assert.True(t, stale, "a stale doc must be reported out of date")
	before, err := fs.ReadFile(os.DirFS(dir), "README.md")
	require.NoError(t, err)
	assert.NotContains(t, string(before), "a();", "check must not write")

	// Generate fills it in.
	changed, err := snippets.Generate(docPath)
	require.NoError(t, err)
	assert.True(t, changed)
	after, err := fs.ReadFile(os.DirFS(dir), "README.md")
	require.NoError(t, err)
	assert.Contains(t, string(after), "```js\na();\n```")

	// Now it is up to date: neither reports a change.
	stale, err = snippets.Check(docPath)
	require.NoError(t, err)
	assert.False(t, stale, "an up-to-date doc must not be reported stale")
	changed, err = snippets.Generate(docPath)
	require.NoError(t, err)
	assert.False(t, changed, "regenerating an up-to-date doc must be a no-op")
}
