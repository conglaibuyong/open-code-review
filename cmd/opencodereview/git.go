package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/open-code-review/open-code-review/internal/vcs"
)

// runGitCmd executes a git command and returns combined output.
// Deprecated: Use vcs.Provider interface instead.
func runGitCmd(repoDir string, args ...string) ([]byte, error) {
	fullArgs := append([]string{"-C", repoDir}, args...)
	cmd := exec.Command("git", fullArgs...)
	return cmd.CombinedOutput()
}

// getCommitMessage returns the commit message for the given ref.
// Deprecated: Use vcs.Provider.GetCommitMessage instead.
func getCommitMessage(repoDir, commit string) (string, error) {
	out, err := runGitCmd(repoDir, "log", "-1", "--format=%B", commit)
	if err != nil {
		return "", fmt.Errorf("git log failed: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// resolveRepoDir resolves and validates a git repository directory.
// Deprecated: Use vcs.Provider.ResolveRepoDir instead.
func resolveRepoDir(input string) (string, error) {
	if input == "" {
		var err error
		input, err = os.Getwd()
		if err != nil {
			return "", fmt.Errorf("get working directory: %w", err)
		}
	}
	absPath, err := filepath.Abs(input)
	if err != nil {
		return "", fmt.Errorf("resolve absolute path: %w", err)
	}
	out, err := runGitCmd(absPath, "rev-parse", "--git-dir")
	if err != nil || len(out) == 0 {
		return "", fmt.Errorf("%s is not a git repository", absPath)
	}
	return absPath, nil
}

// requireGitRepo validates that the given directory is part of a git repository.
// Deprecated: Use vcs.GitProvider.Detect instead.
func requireGitRepo(dir string) error {
	repoDir, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}
	out, err := runGitCmd(repoDir, "rev-parse", "--git-dir")
	if err != nil || len(out) == 0 {
		return fmt.Errorf("%s is not a git repository, code review requires a valid git repository", repoDir)
	}
	return nil
}

// resolveVCSBackend detects the VCS type for the given directory.
func resolveVCSBackendAuto(dir string) (vcs.Backend, error) {
	return vcs.DetectBackend(dir)
}
