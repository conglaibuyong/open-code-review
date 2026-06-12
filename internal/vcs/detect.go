package vcs

import (
	"fmt"
	"os"
	"path/filepath"
)

// DetectBackend auto-detects the VCS backend used in the given directory.
// It checks for Git first, then TFVC. Returns the detected Backend or an error
// if no known VCS is found.
func DetectBackend(dir string) (Backend, error) {
	absPath, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("resolve absolute path: %w", err)
	}

	// Check for Git repository (.git directory or file)
	if isGitRepo(absPath) {
		return Git, nil
	}

	// Check for TFVC workspace ($tf directory)
	if isTFVCWorkspace(absPath) {
		return TFVC, nil
	}

	return "", fmt.Errorf("%s is not a recognized VCS repository (neither Git nor TFVC)", absPath)
}

// isGitRepo checks if the directory is part of a Git repository.
func isGitRepo(dir string) bool {
	// Check for .git directory or .git file (worktree)
	gitPath := filepath.Join(dir, ".git")
	if _, err := os.Stat(gitPath); err == nil {
		return true
	}

	// Walk up parent directories to find .git
	parent := filepath.Dir(dir)
	for parent != dir {
		gitPath := filepath.Join(parent, ".git")
		if _, err := os.Stat(gitPath); err == nil {
			return true
		}
		dir = parent
		parent = filepath.Dir(dir)
	}

	return false
}

// isTFVCWorkspace checks if the directory is part of a TFVC workspace.
// TFVC local workspaces have a $tf directory at the workspace root.
func isTFVCWorkspace(dir string) bool {
	// Check for $tf directory (TFVC local workspace marker)
	tfPath := filepath.Join(dir, "$tf")
	if info, err := os.Stat(tfPath); err == nil && info.IsDir() {
		return true
	}

	// Walk up parent directories to find $tf
	parent := filepath.Dir(dir)
	for parent != dir {
		tfPath := filepath.Join(parent, "$tf")
		if info, err := os.Stat(tfPath); err == nil && info.IsDir() {
			return true
		}
		dir = parent
		parent = filepath.Dir(dir)
	}

	return false
}
