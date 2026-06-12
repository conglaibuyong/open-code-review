// Package diff parses unified git diff output into structured Diff objects.
package diff

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/open-code-review/open-code-review/internal/model"
	"github.com/open-code-review/open-code-review/internal/vcs"
)

var (
	diffHeaderRe = regexp.MustCompile(`^diff --git a/(.+?) b/(.+)$`)
	oldFileRe    = regexp.MustCompile(`^--- a/(.+)$`)
	newFileRe    = regexp.MustCompile(`^\+\+\+ b/(.+)$`)
	binaryRe     = regexp.MustCompile(`Binary files `)
)

// ParseDiffText splits the unified diff text into per-file Diff structs.
// ref, if non-empty, is a VCS ref used to read new-file content.
// vcsProv, if non-nil, is used to read file content at the given ref.
func ParseDiffText(ctx context.Context, diffText string, repoDir string, ref string, vcsProv vcs.Provider) ([]model.Diff, error) {
	lines := strings.Split(diffText, "\n")
	var diffs []model.Diff
	var current *model.Diff
	var buf strings.Builder

	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	for _, line := range lines {
		if m := diffHeaderRe.FindStringSubmatch(line); m != nil {
			// Flush previous diff
			if current != nil {
				current.Diff = strings.TrimSuffix(buf.String(), "\n")
				finalizeDiff(ctx, current, repoDir, ref, vcsProv)
				diffs = append(diffs, *current)
				buf.Reset()
			}
			current = &model.Diff{
				OldPath: m[1],
				NewPath: m[2],
			}
		}
		if current == nil {
			continue
		}

		switch {
		case binaryRe.MatchString(line):
			current.IsBinary = true
		case oldFileRe.MatchString(line):
			if p := oldFileRe.FindStringSubmatch(line); len(p) > 1 && p[1] == "/dev/null" {
				current.IsNew = true
			}
		case newFileRe.MatchString(line):
			if p := newFileRe.FindStringSubmatch(line); len(p) > 1 && p[1] == "/dev/null" {
				current.IsDeleted = true
			}
		case strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++"):
			current.Insertions++
		case strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---"):
			current.Deletions++
		}
		buf.WriteString(line)
		buf.WriteString("\n")
	}

	// Flush last diff
	if current != nil {
		current.Diff = strings.TrimSuffix(buf.String(), "\n")
		finalizeDiff(ctx, current, repoDir, ref, vcsProv)
		diffs = append(diffs, *current)
	}

	return diffs, nil
}

// finalizeDiff reads the new file content. When ref is non-empty it uses
// the VCS provider to read the file at that ref; otherwise it reads from disk.
func finalizeDiff(ctx context.Context, d *model.Diff, repoDir string, ref string, vcsProv vcs.Provider) {
	if d.IsDeleted || d.NewPath == "/dev/null" {
		d.NewPath = "/dev/null"
		return
	}
	if ref != "" && vcsProv != nil {
		output, err := vcsProv.ReadFile(ctx, repoDir, d.NewPath, ref)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[ocr] WARNING: cannot read file %s at ref %s: %v\n",
				d.NewPath, ref, err)
			return
		}
		d.NewFileContent = string(output)
		return
	}
	fullPath := filepath.Join(repoDir, d.NewPath)
	content, err := os.ReadFile(fullPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ocr] WARNING: cannot read file %s for review: %v\n", d.NewPath, err)
		return
	}
	d.NewFileContent = string(content)
}
