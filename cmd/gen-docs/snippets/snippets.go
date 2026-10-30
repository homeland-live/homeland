// Package snippets embeds source files into a Markdown document between
// gen-docs markers, so documentation code blocks stay in sync with the real,
// type-checked files they show. It backs the gen-docs build-time tool and is
// not part of the server.
package snippets

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

// beginMarker names a file path or a glob pattern relative to the document's
// directory; its region ends at the next endMarker. Everything between is
// replaced. Markers sit on their own line:
//
//	<!-- gen-docs:begin examples/foo.js -->
//	```js
//	...file contents...
//	```
//	<!-- gen-docs:end -->
//
// A pattern (path.Match syntax, such as examples/*.js) renders every matching
// regular, non-hidden file in lexical order, each under a level-3 heading of
// its file name, separated by a horizontal rule:
//
//	<!-- gen-docs:begin examples/*.js -->
//	### a.js
//
//	```js
//	...a.js contents...
//	```
//
//	---
//
//	### b.js
//	...
//	<!-- gen-docs:end -->
var (
	beginMarker = regexp.MustCompile(`^<!-- gen-docs:begin (\S+) -->$`)
	endMarker   = regexp.MustCompile(`^<!-- gen-docs:end -->$`)
)

// lang maps a file extension to the fenced code block's language label.
var lang = map[string]string{
	".js":   "js",
	".ts":   "ts",
	".json": "json",
	".go":   "go",
	".sh":   "sh",
	".yaml": "yaml",
	".yml":  "yaml",
}

// Render returns doc with every gen-docs region's body replaced: a file path
// becomes a fenced code block of that file, and a glob pattern becomes a
// heading and fenced code block per matching file. Files are read relative to
// baseDir through an os.Root, so a path can't escape it. The markers
// themselves are preserved, so Render is idempotent, and an empty region is
// filled in place. It fails on a begin without a matching end, a nested
// begin, a stray end, a missing, unreadable or escaping file, or a malformed,
// escaping or empty-matching pattern, naming the offending marker or path. A document with
// no markers is returned unchanged.
//
// The document's final newline (if any) is preserved.
func Render(doc []byte, baseDir string) ([]byte, error) {
	root, err := os.OpenRoot(baseDir)
	if err != nil {
		return nil, fmt.Errorf("gen-docs/snippets: open %q: %w", baseDir, err)
	}
	defer func() { _ = root.Close() }()

	trailingNewline := bytes.HasSuffix(doc, []byte("\n"))
	lines := strings.Split(strings.TrimSuffix(string(doc), "\n"), "\n")

	var out []string
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		m := beginMarker.FindStringSubmatch(line)
		if m == nil {
			if endMarker.MatchString(line) {
				return nil, fmt.Errorf("gen-docs/snippets: gen-docs:end without a matching begin at line %d", i+1)
			}
			out = append(out, line)
			continue
		}

		// Inside a region: emit the begin marker and the rendered body, then
		// skip the old body up to and including the end marker.
		spec := m[1]
		body, err := renderRegion(root, spec)
		if err != nil {
			return nil, err
		}
		out = append(out, line)
		out = append(out, body...)

		i++
		for ; i < len(lines); i++ {
			if endMarker.MatchString(lines[i]) {
				break
			}
			if b := beginMarker.FindStringSubmatch(lines[i]); b != nil {
				return nil, fmt.Errorf("gen-docs/snippets: gen-docs:begin %q nested inside %q (missing end?)", b[1], spec)
			}
		}
		if i == len(lines) {
			return nil, fmt.Errorf("gen-docs/snippets: gen-docs:begin %q has no matching end", spec)
		}
		out = append(out, lines[i]) // the end marker
	}

	result := strings.Join(out, "\n")
	if trailingNewline {
		result += "\n"
	}
	return []byte(result), nil
}

// renderRegion returns the body lines for a region whose begin marker names
// spec: a single fenced block for a plain path, or a heading and fenced block
// per matching file for a glob pattern.
func renderRegion(root *os.Root, spec string) ([]string, error) {
	if !strings.ContainsAny(spec, `*?[\`) {
		return fencedFile(root, spec)
	}
	// Clean so ./examples/*.js works like ./examples/a.js does through os.Root;
	// a pattern still starting with .. or / is rejected.
	spec = path.Clean(spec)
	if !fs.ValidPath(spec) {
		return nil, fmt.Errorf("gen-docs/snippets: pattern %q escapes the base directory", spec)
	}
	fsys := root.FS()
	matches, err := fs.Glob(fsys, spec)
	if err != nil {
		return nil, fmt.Errorf("gen-docs/snippets: glob %q: %w", spec, err)
	}

	var out []string
	for _, name := range matches {
		base := path.Base(name)
		if strings.HasPrefix(base, ".") {
			continue
		}
		info, err := fs.Stat(fsys, name)
		if err != nil {
			return nil, fmt.Errorf("gen-docs/snippets: stat %q: %w", name, err)
		}
		if !info.Mode().IsRegular() {
			continue
		}
		block, err := fencedFile(root, name)
		if err != nil {
			return nil, err
		}
		if len(out) > 0 {
			out = append(out, "", "---", "")
		}
		out = append(out, "### "+base, "")
		out = append(out, block...)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("gen-docs/snippets: pattern %q matches no files", spec)
	}
	return out, nil
}

// fencedFile reads rel inside root and returns its fenced code block as lines
// (no trailing newline). The file's content is trimmed of surrounding blank
// lines so the block is tight.
func fencedFile(root *os.Root, rel string) ([]string, error) {
	content, err := root.ReadFile(filepath.FromSlash(rel))
	if err != nil {
		return nil, fmt.Errorf("gen-docs/snippets: read %q: %w", rel, err)
	}
	label := lang[strings.ToLower(filepath.Ext(rel))]
	trimmed := strings.Trim(string(content), "\n")

	out := []string{"```" + label}
	out = append(out, strings.Split(trimmed, "\n")...)
	out = append(out, "```")
	return out, nil
}

// Generate rewrites docPath in place from the files its markers reference
// (resolved relative to docPath's directory) and reports whether it changed.
// An up-to-date file is left untouched.
func Generate(docPath string) (changed bool, err error) {
	return update(docPath, true)
}

// Check reports whether docPath is out of date, without writing it, so a
// caller can fail CI. The error is nil unless reading or rendering failed.
func Check(docPath string) (stale bool, err error) {
	return update(docPath, false)
}

// update renders docPath and reports whether the result differs from the
// file, writing it (with the file's permissions) when write is set.
func update(docPath string, write bool) (bool, error) {
	dir, name := filepath.Split(docPath)
	if dir == "" {
		dir = "."
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return false, fmt.Errorf("gen-docs/snippets: open %q: %w", dir, err)
	}
	defer func() { _ = root.Close() }()

	doc, err := root.ReadFile(name)
	if err != nil {
		return false, fmt.Errorf("gen-docs/snippets: read %q: %w", docPath, err)
	}

	rendered, err := Render(doc, dir)
	if err != nil {
		return false, err
	}

	if bytes.Equal(doc, rendered) {
		return false, nil
	}
	if !write {
		return true, nil
	}

	info, err := root.Stat(name)
	if err != nil {
		return false, fmt.Errorf("gen-docs/snippets: stat %q: %w", docPath, err)
	}
	if err := root.WriteFile(name, rendered, info.Mode().Perm()); err != nil {
		return false, fmt.Errorf("gen-docs/snippets: write %q: %w", docPath, err)
	}
	return true, nil
}
