package mdshare

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Server-side limits, checked here so the upload fails before it starts.
const (
	MaxBytes   = 5242880
	MaxFiles   = 500
	MaxPathLen = 512
)

// IsMarkdown reports whether the file name has an extension the server accepts.
func IsMarkdown(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".md", ".mdx", ".markdown":
		return true
	}
	return false
}

// Collect turns CLI arguments into the files of a share. Directories are
// walked recursively and keep their name as the first path segment; "-" reads
// one document from stdin and names it stdinName.
func Collect(args []string, stdin io.Reader, stdinName string) ([]File, error) {
	var files []File
	seen := map[string]bool{}
	total := 0

	add := func(path, content, source string) error {
		if !utf8.ValidString(content) {
			return fmt.Errorf("%s is not valid UTF-8", source)
		}
		if seen[path] {
			return fmt.Errorf("duplicate path %q (from %s)", path, source)
		}
		if len(path) > MaxPathLen {
			return fmt.Errorf("path %q is longer than %d characters", path, MaxPathLen)
		}
		seen[path] = true
		total += len(content)
		files = append(files, File{Path: path, Content: content})
		return nil
	}

	addFile := func(fsPath, sharePath string) error {
		data, err := os.ReadFile(fsPath)
		if err != nil {
			return err
		}
		return add(sharePath, string(data), fsPath)
	}

	for _, arg := range args {
		if arg == "-" {
			if !IsMarkdown(stdinName) {
				return nil, fmt.Errorf("stdin name %q must end in .md, .mdx or .markdown", stdinName)
			}
			data, err := io.ReadAll(stdin)
			if err != nil {
				return nil, fmt.Errorf("read stdin: %w", err)
			}
			if err := add(filepath.ToSlash(stdinName), string(data), "stdin"); err != nil {
				return nil, err
			}
			continue
		}

		info, err := os.Stat(arg)
		if err != nil {
			return nil, err
		}

		if !info.IsDir() {
			if !IsMarkdown(arg) {
				return nil, fmt.Errorf("%s is not a markdown file (.md, .mdx, .markdown)", arg)
			}
			if err := addFile(arg, filepath.Base(arg)); err != nil {
				return nil, err
			}
			continue
		}

		// Abs so that "." and "docs/" resolve to a real folder name.
		root, err := filepath.Abs(arg)
		if err != nil {
			return nil, err
		}
		base := filepath.Base(root)
		err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if p != root && (strings.HasPrefix(d.Name(), ".") || d.Name() == "node_modules") {
					return filepath.SkipDir
				}
				return nil
			}
			// Symlinks are not followed: they can point outside the folder.
			if !d.Type().IsRegular() || !IsMarkdown(d.Name()) {
				return nil
			}
			rel, err := filepath.Rel(root, p)
			if err != nil {
				return err
			}
			return addFile(p, filepath.ToSlash(filepath.Join(base, rel)))
		})
		if err != nil {
			return nil, err
		}
	}

	if len(files) == 0 {
		return nil, fmt.Errorf("no markdown files found")
	}
	if len(files) > MaxFiles {
		return nil, fmt.Errorf("%d files, the limit is %d", len(files), MaxFiles)
	}
	if total > MaxBytes {
		return nil, fmt.Errorf("%s of markdown, the limit is %s (%s over)",
			FormatBytes(total), FormatBytes(MaxBytes), FormatBytes(total-MaxBytes))
	}
	return files, nil
}

var imageRef = regexp.MustCompile(`!\[[^\]]*\]\(\s*<?([^)\s>]+)`)

// LocalImageRefs counts image links that point at local files, which a share
// cannot serve.
func LocalImageRefs(files []File) int {
	n := 0
	for _, f := range files {
		for _, m := range imageRef.FindAllStringSubmatch(f.Content, -1) {
			target := strings.ToLower(m[1])
			if strings.Contains(target, "://") || strings.HasPrefix(target, "//") || strings.HasPrefix(target, "data:") {
				continue
			}
			n++
		}
	}
	return n
}

func FormatBytes(n int) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d B", n)
}
