package vcs

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/open-code-review/open-code-review/internal/gitcmd"
)

// GitProvider implements the Provider interface for Git repositories.
type GitProvider struct {
	runner *gitcmd.Runner
}

// NewGitProvider creates a GitProvider with the given concurrency limit.
// If maxConcurrent <= 0, the default (16) is used.
func NewGitProvider(maxConcurrent int) *GitProvider {
	return &GitProvider{
		runner: gitcmd.New(maxConcurrent),
	}
}

// NewGitProviderWithRunner creates a GitProvider with an existing Runner.
func NewGitProviderWithRunner(runner *gitcmd.Runner) *GitProvider {
	return &GitProvider{runner: runner}
}

// Runner returns the underlying gitcmd.Runner for backward compatibility.
func (g *GitProvider) Runner() *gitcmd.Runner {
	return g.runner
}

func (g *GitProvider) Name() Backend { return Git }

func (g *GitProvider) Detect(repoDir string) bool {
	out, err := g.runGit(repoDir, "rev-parse", "--git-dir")
	return err == nil && len(out) > 0
}

func (g *GitProvider) ResolveRepoDir(dir string) (string, error) {
	absPath, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("resolve absolute path: %w", err)
	}
	out, err := g.runGit(absPath, "rev-parse", "--git-dir")
	if err != nil || len(out) == 0 {
		return "", fmt.Errorf("%s is not a git repository", absPath)
	}
	return absPath, nil
}

func (g *GitProvider) GetDiff(ctx context.Context, opts DiffOptions) (string, error) {
	contextLines := opts.Context
	if contextLines <= 0 {
		contextLines = 3
	}
	uFlag := fmt.Sprintf("-U%d", contextLines)

	switch opts.Mode {
	case ModeRange:
		return g.getRangeDiff(ctx, opts, uFlag)
	case ModeCommit:
		return g.getCommitDiff(ctx, opts, uFlag)
	case ModeWorkspace:
		return g.getWorkspaceDiff(ctx, opts, uFlag)
	case ModeShelveset:
		return "", fmt.Errorf("shelveset mode is not supported by Git")
	default:
		return "", fmt.Errorf("unknown diff mode: %d", opts.Mode)
	}
}

func (g *GitProvider) getRangeDiff(ctx context.Context, opts DiffOptions, uFlag string) (string, error) {
	// Compute merge-base
	base, err := g.runner.Run(ctx, opts.RepoDir, "merge-base", opts.From, opts.To)
	if err != nil {
		return "", fmt.Errorf("git merge-base failed: %w", err)
	}
	base = strings.TrimSpace(base)
	if base == "" {
		return "", fmt.Errorf("cannot find merge-base between %s and %s", opts.From, opts.To)
	}

	out, err := g.runner.Run(ctx, opts.RepoDir,
		"diff", "--no-ext-diff", "--no-textconv",
		"--src-prefix=a/", "--dst-prefix=b/", "--no-color",
		uFlag, base, opts.To, "--")
	if err != nil {
		return "", fmt.Errorf("git diff failed: %w", err)
	}
	return out, nil
}

func (g *GitProvider) getCommitDiff(ctx context.Context, opts DiffOptions, uFlag string) (string, error) {
	out, err := g.runner.Run(ctx, opts.RepoDir,
		"show", "--no-ext-diff", "--no-textconv",
		"--src-prefix=a/", "--dst-prefix=b/", "--no-color",
		uFlag, opts.Commit)
	if err != nil {
		return "", fmt.Errorf("git show failed: %w", err)
	}
	return out, nil
}

func (g *GitProvider) getWorkspaceDiff(ctx context.Context, opts DiffOptions, uFlag string) (string, error) {
	var combined strings.Builder

	// Tracked changes (HEAD vs working tree, fallback to staged)
	tracked, err := g.runner.Run(ctx, opts.RepoDir,
		"diff", "--no-ext-diff", "--no-textconv",
		"--src-prefix=a/", "--dst-prefix=b/", "HEAD",
		"--no-color", uFlag, "--")
	if err == nil && tracked != "" {
		combined.WriteString(tracked)
	} else if ctx.Err() != nil {
		return "", ctx.Err()
	} else {
		// Fallback to staged only
		staged, err := g.runner.Run(ctx, opts.RepoDir,
			"diff", "--no-ext-diff", "--no-textconv",
			"--src-prefix=a/", "--dst-prefix=b/", "--staged",
			"--no-color", uFlag, "--")
		if err != nil {
			return "", fmt.Errorf("workspace tracked diff failed: %w", err)
		}
		combined.WriteString(staged)
	}

	// Untracked files
	untracked, err := g.getUntrackedDiffs(ctx, opts.RepoDir)
	if err != nil {
		return "", fmt.Errorf("untracked file diff failed: %w", err)
	}
	for _, ud := range untracked {
		combined.WriteString(ud)
		combined.WriteString("\n\n")
	}

	return combined.String(), nil
}

func (g *GitProvider) getUntrackedDiffs(ctx context.Context, repoDir string) ([]string, error) {
	out, err := g.runner.Run(ctx, repoDir, "ls-files", "--others", "--exclude-standard")
	if err != nil || out == "" {
		return nil, nil
	}

	var results []string
	for _, f := range strings.Split(strings.TrimSpace(out), "\n") {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		results = append(results, formatNewFileDiff(f, repoDir))
	}
	return results, nil
}

func (g *GitProvider) ReadFile(ctx context.Context, repoDir string, path string, ref string) ([]byte, error) {
	if ref == "" {
		// Workspace mode: read from disk
		fullPath := filepath.Join(repoDir, path)
		return exec.CommandContext(ctx, "cat", fullPath).Output()
	}

	// Range/Commit mode: use git show
	args := []string{"-c", "core.quotepath=false", "show", ref + ":" + path}
	if g.runner != nil {
		return g.runner.Output(ctx, repoDir, args...)
	}
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = repoDir
	return cmd.Output()
}

func (g *GitProvider) GetCurrentBranch(repoDir string) string {
	out, err := g.runGit(repoDir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil || len(out) == 0 {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func (g *GitProvider) GetCommitMessage(repoDir string, ref string) (string, error) {
	out, err := g.runGit(repoDir, "log", "-1", "--format=%B", ref)
	if err != nil {
		return "", fmt.Errorf("git log failed: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func (g *GitProvider) IgnoreFile() string {
	return ".gitignore"
}

func (g *GitProvider) runGit(repoDir string, args ...string) ([]byte, error) {
	fullArgs := append([]string{"-C", repoDir}, args...)
	cmd := exec.Command("git", fullArgs...)
	return cmd.CombinedOutput()
}

// MergeBase computes the merge-base commit between two refs.
// This is a Git-specific operation exposed for backward compatibility.
func (g *GitProvider) MergeBase(ctx context.Context, repoDir, from, to string) (string, error) {
	out, err := g.runner.Run(ctx, repoDir, "merge-base", from, to)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// UntrackedFilesList returns the list of untracked files in the repository.
// This is a Git-specific operation exposed for backward compatibility.
func (g *GitProvider) UntrackedFilesList(ctx context.Context, repoDir string) ([]string, error) {
	out, err := g.runner.Run(ctx, repoDir, "ls-files", "--others", "--exclude-standard")
	if err != nil || out == "" {
		return nil, nil
	}
	var files []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			files = append(files, line)
		}
	}
	return files, nil
}
