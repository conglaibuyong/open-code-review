package tool

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/open-code-review/open-code-review/internal/gitcmd"
	"github.com/open-code-review/open-code-review/internal/vcs"
)

// ReviewMode represents the active review mode.
type ReviewMode int

const (
	// ModeWorkspace reads files from the current working tree.
	ModeWorkspace ReviewMode = iota
	// ModeRange reads files as they exist at a specific VCS ref (--to value).
	ModeRange
	// ModeCommit reads files as they exist at a specific commit/changeset.
	ModeCommit
	// ModeShelveset reads files from a TFVC shelveset.
	ModeShelveset
)

// ParseReviewMode returns the correct ReviewMode based on provided flag values.
func ParseReviewMode(from, to, commit, shelveset string) ReviewMode {
	if shelveset != "" {
		return ModeShelveset
	}
	if commit != "" {
		return ModeCommit
	}
	if from != "" && to != "" {
		return ModeRange
	}
	return ModeWorkspace
}

// RefValue returns the VCS ref that should be used for reading file contents
// in range or commit mode. Returns ("", false) for workspace/shelveset mode.
func (m ReviewMode) RefValue(toRef, commit string) (string, bool) {
	switch m {
	case ModeRange:
		return toRef, true
	case ModeCommit:
		return commit, true
	default:
		return "", false
	}
}

// FileReader resolves file contents according to the active review mode.
type FileReader struct {
	RepoDir string
	Mode    ReviewMode
	// Ref is the VCS ref to use for ModeRange (--to) or ModeCommit (--commit).
	// Empty for ModeWorkspace or ModeShelveset.
	Ref     string
	VCSProv vcs.Provider
	// Runner is kept for backward compatibility with code_search.go git grep.
	// When nil, git commands are executed directly.
	Runner *gitcmd.Runner
}

// Read returns the full content of a file path (relative to RepoDir),
// resolved according to the active review mode.
// - Workspace / Shelveset: reads directly from the filesystem.
// - Range / Commit: uses VCS provider to read at the given ref.
func (fr *FileReader) Read(ctx context.Context, path string) (string, error) {
	switch fr.Mode {
	case ModeWorkspace, ModeShelveset:
		return fr.readFromDisk(path)
	case ModeRange, ModeCommit:
		return fr.readFromVCS(ctx, path)
	default:
		return fr.readFromDisk(path)
	}
}

func (fr *FileReader) readFromDisk(path string) (string, error) {
	fullPath := filepath.Join(fr.RepoDir, path)
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return "", fmt.Errorf("read file %q: %w", path, err)
	}
	return string(content), nil
}

func (fr *FileReader) readFromVCS(ctx context.Context, path string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if fr.VCSProv != nil {
		output, err := fr.VCSProv.ReadFile(ctx, fr.RepoDir, path, fr.Ref)
		if err != nil {
			return "", fmt.Errorf("read file %s at ref %s: %w", path, fr.Ref, err)
		}
		return string(output), nil
	}

	// Fallback: should not happen, but preserve backward compatibility
	return fr.readFromDisk(path)
}

// ReadLines returns a window of lines from the file plus the total line count.
// startLine is 1-based; maxLines is the maximum number of lines to collect.
func (fr *FileReader) ReadLines(ctx context.Context, path string, startLine, maxLines int) ([]string, int, error) {
	switch fr.Mode {
	case ModeWorkspace, ModeShelveset:
		return fr.readLinesFromDisk(path, startLine, maxLines)
	case ModeRange, ModeCommit:
		innerCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		return fr.readLinesFromVCS(innerCtx, path, startLine, maxLines)
	default:
		return fr.readLinesFromDisk(path, startLine, maxLines)
	}
}

// scanLines reads from r line by line, collecting at most maxLines lines
// starting from startLine (1-based), while counting the total number of lines.
// The behavior matches strings.Split(content, "\n") for trailing-newline files.
func scanLines(r io.Reader, startLine, maxLines int) ([]string, int, error) {
	br := bufio.NewReader(r)
	var collected []string
	lineNum := 0
	lastHadNewline := false

	for {
		line, err := br.ReadString('\n')
		if len(line) > 0 {
			lineNum++
			lastHadNewline = line[len(line)-1] == '\n'
			trimmed := strings.TrimSuffix(line, "\n")
			trimmed = strings.TrimSuffix(trimmed, "\r")
			if lineNum >= startLine && len(collected) < maxLines {
				collected = append(collected, trimmed)
			}
		}
		if err != nil {
			if err != io.EOF {
				return nil, 0, err
			}
			break
		}
	}

	if lastHadNewline {
		lineNum++
		if lineNum >= startLine && len(collected) < maxLines {
			collected = append(collected, "")
		}
	}

	return collected, lineNum, nil
}

func (fr *FileReader) readLinesFromDisk(path string, startLine, maxLines int) ([]string, int, error) {
	fullPath := filepath.Join(fr.RepoDir, path)
	f, err := os.Open(fullPath)
	if err != nil {
		return nil, 0, fmt.Errorf("read file %q: %w", path, err)
	}
	defer f.Close()

	return scanLines(f, startLine, maxLines)
}

func (fr *FileReader) readLinesFromVCS(ctx context.Context, path string, startLine, maxLines int) ([]string, int, error) {
	if fr.VCSProv == nil {
		return fr.readLinesFromDisk(path, startLine, maxLines)
	}

	output, err := fr.VCSProv.ReadFile(ctx, fr.RepoDir, path, fr.Ref)
	if err != nil {
		return nil, 0, fmt.Errorf("read file %s at ref %s: %w", path, fr.Ref, err)
	}

	return scanLines(strings.NewReader(string(output)), startLine, maxLines)
}
