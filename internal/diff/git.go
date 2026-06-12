package diff

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/open-code-review/open-code-review/internal/model"
	"github.com/open-code-review/open-code-review/internal/vcs"
)

// DiffContextLines defines the number of context lines around each changed hunk.
const DiffContextLines = 3

// providerDirIgnoreDirs: directory prefixes to always exclude from diff results.
var providerDirIgnoreDirs = []string{
	".idea/",
	".vscode/",
	".svn/",
	".git/",
	"$tf/",
	"vendor/",
	"node_modules/",
	"target/",
	".happypack/",
	".cachefile/",
	"_packages/",
	"rpm/",
	"pkgs/",
}

// Mode defines how the diff is retrieved.
type Mode int

const (
	ModeWorkspace Mode = iota // current workspace (staged + unstaged + untracked)
	ModeCommit                // single commit vs its parent
	ModeRange                 // merge-base(from,to)..to
	ModeShelveset             // TFVC shelveset
)

// Provider retrieves and parses diffs from a repository using a VCS backend.
type Provider struct {
	repoDir string
	mode    Mode
	vcsProv vcs.Provider

	// Range mode parameters
	from, to string // from/to refs for range comparison

	// Commit mode parameter
	commit string // single commit hash/ref

	// Shelveset mode parameter (TFVC-specific)
	shelveset string

	mergeBase string // cached common ancestor for range mode (Git-specific)
}

// NewProvider creates a Provider for range mode: from..to (via merge-base).
func NewProvider(repoDir, from, to string, vcsProv vcs.Provider) *Provider {
	return &Provider{
		repoDir: repoDir,
		mode:    ModeRange,
		from:    from,
		to:      to,
		vcsProv: vcsProv,
	}
}

// NewCommitProvider creates a Provider for commit mode: show changes introduced by a single commit.
func NewCommitProvider(repoDir, commit string, vcsProv vcs.Provider) *Provider {
	return &Provider{
		repoDir: repoDir,
		mode:    ModeCommit,
		commit:  commit,
		vcsProv: vcsProv,
	}
}

// NewWorkspaceProvider creates a Provider for workspace mode (current uncommitted changes).
func NewWorkspaceProvider(repoDir string, vcsProv vcs.Provider) *Provider {
	return &Provider{
		repoDir: repoDir,
		mode:    ModeWorkspace,
		vcsProv: vcsProv,
	}
}

// NewShelvesetProvider creates a Provider for shelveset mode (TFVC-specific).
func NewShelvesetProvider(repoDir, shelveset string, vcsProv vcs.Provider) *Provider {
	return &Provider{
		repoDir:   repoDir,
		mode:      ModeShelveset,
		shelveset: shelveset,
		vcsProv:   vcsProv,
	}
}

// IsRangeMode returns true when comparing two refs.
func (p *Provider) IsRangeMode() bool {
	return p.mode == ModeRange
}

// IsCommitMode returns true when analyzing a single commit.
func (p *Provider) IsCommitMode() bool {
	return p.mode == ModeCommit
}

// IsShelvesetMode returns true when analyzing a shelveset.
func (p *Provider) IsShelvesetMode() bool {
	return p.mode == ModeShelveset
}

// MergeBase returns the computed merge-base commit hash for range mode (Git-specific).
func (p *Provider) MergeBase(ctx context.Context) string {
	if p.mode != ModeRange || p.mergeBase != "" {
		return p.mergeBase
	}
	p.mergeBase = p.computeMergeBase(ctx, p.from, p.to)
	return p.mergeBase
}

// GetDiff returns all changes as parsed model.Diff structs.
func (p *Provider) GetDiff(ctx context.Context) ([]model.Diff, error) {
	diffOpts := vcs.DiffOptions{
		Mode:      p.toVCSMode(),
		RepoDir:   p.repoDir,
		From:      p.from,
		To:        p.to,
		Commit:    p.commit,
		Shelveset: p.shelveset,
		Context:   DiffContextLines,
	}

	diffText, err := p.vcsProv.GetDiff(ctx, diffOpts)
	if err != nil {
		return nil, err
	}

	var ref string
	switch p.mode {
	case ModeRange:
		ref = p.to
	case ModeCommit:
		ref = p.commit
	}

	diffs, err := ParseDiffText(ctx, diffText, p.repoDir, ref, p.vcsProv)
	if err != nil {
		return nil, err
	}
	return p.filterDiffs(diffs), nil
}

// toVCSMode converts the internal Mode to vcs.DiffMode.
func (p *Provider) toVCSMode() vcs.DiffMode {
	switch p.mode {
	case ModeWorkspace:
		return vcs.ModeWorkspace
	case ModeCommit:
		return vcs.ModeCommit
	case ModeRange:
		return vcs.ModeRange
	case ModeShelveset:
		return vcs.ModeShelveset
	default:
		return vcs.ModeWorkspace
	}
}

// loadIgnorePatterns reads and parses ignore patterns from the VCS-specific ignore file.
func (p *Provider) loadIgnorePatterns() []string {
	ignoreFile := p.vcsProv.IgnoreFile()
	data, err := os.ReadFile(filepath.Join(p.repoDir, ignoreFile))
	if err != nil {
		// Also try .gitignore as fallback for TFVC repos that may have both
		if ignoreFile != ".gitignore" {
			if data2, err2 := os.ReadFile(filepath.Join(p.repoDir, ".gitignore")); err2 == nil {
				return parseIgnorePatterns(string(data2))
			}
		}
		return nil
	}
	return parseIgnorePatterns(string(data))
}

// parseIgnorePatterns parses ignore file content into pattern lines.
func parseIgnorePatterns(content string) []string {
	var patterns []string
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, line)
	}
	return patterns
}

// isPathExcluded returns true when the given relative file path should be skipped
// based on hardcoded dir rules or ignore patterns.
func (p *Provider) isPathExcluded(relPath string, ignorePatterns []string) bool {
	// Hardcoded directory prefix checks
	for _, prefix := range providerDirIgnoreDirs {
		dirPart := strings.TrimSuffix(prefix, "/")
		if relPath == dirPart || strings.HasPrefix(relPath, prefix) {
			return true
		}
	}

	// Ignore file pattern matching
	for _, pat := range ignorePatterns {
		if matchIgnorePattern(relPath, pat) {
			return true
		}
	}
	return false
}

// matchIgnorePattern checks if relPath matches a single ignore pattern.
func matchIgnorePattern(relPath, pat string) bool {
	// Directory-only patterns (trailing /)
	if strings.HasSuffix(pat, "/") {
		dirName := strings.TrimSuffix(pat, "/")
		// Match if any path segment equals the dir name
		segments := strings.Split(relPath, "/")
		for _, seg := range segments {
			if seg == dirName {
				return true
			}
		}
		return false
	}

	// Negation patterns are not needed for exclusion purposes
	if strings.HasPrefix(pat, "!") {
		return false
	}

	// Patterns without / match basename
	if !strings.Contains(pat, "/") {
		base := filepath.Base(relPath)
		if matched, _ := filepath.Match(pat, base); matched {
			return true
		}
		return false
	}

	// Patterns with / match against the full relative path
	if matched, _ := filepath.Match(pat, relPath); matched {
		return true
	}
	// Also try matching against suffix of path
	if strings.HasSuffix(relPath, pat) {
		return true
	}

	return false
}

// filterDiffs removes diffs whose file paths are excluded.
func (p *Provider) filterDiffs(diffs []model.Diff) []model.Diff {
	patterns := p.loadIgnorePatterns()
	var result []model.Diff
	for _, d := range diffs {
		path := d.NewPath
		if path == "/dev/null" {
			path = d.OldPath
		}
		if !p.isPathExcluded(path, patterns) {
			result = append(result, d)
		}
	}
	return result
}

// ---- Internal helpers ----

// computeMergeBase computes the merge-base for Git range mode.
func (p *Provider) computeMergeBase(ctx context.Context, from, to string) string {
	gitProv, ok := p.vcsProv.(*vcs.GitProvider)
	if !ok {
		return "" // merge-base is Git-specific
	}
	base, err := gitProv.MergeBase(ctx, p.repoDir, from, to)
	if err != nil {
		return ""
	}
	return base
}

// formatNewFileDiff constructs a unified diff for a newly added file.
func formatNewFileDiff(relPath string, repoDir string) string {
	fullPath := filepath.Join(repoDir, relPath)
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return ""
	}

	lineCount := bytes.Count(content, []byte{'\n'})
	if len(content) > 0 && content[len(content)-1] != '\n' {
		lineCount++
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("diff --git a/%s b/%s\n", relPath, relPath))
	sb.WriteString("--- /dev/null\n")
	sb.WriteString(fmt.Sprintf("+++ b/%s\n", relPath))
	sb.WriteString(fmt.Sprintf("@@ -0,0 +1,%d @@\n", lineCount))

	lines := bytes.Split(content, []byte{'\n'})
	if len(lines) > 0 && len(lines[len(lines)-1]) == 0 {
		lines = lines[:len(lines)-1]
	}
	for _, line := range lines {
		sb.WriteByte('+')
		sb.Write(line)
		sb.WriteByte('\n')
	}
	return sb.String()
}
