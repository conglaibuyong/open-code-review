package vcs

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

	// Check for TFVC workspace
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
// TFVC has two workspace types:
//   - Local workspace: has a $tf directory at the workspace root
//   - Server workspace: no $tf directory, must use `tf workfold` to verify
//
// This function checks for local workspace markers first (fast, no subprocess),
// then falls back to `tf workfold` for server workspaces.
func isTFVCWorkspace(dir string) bool {
	// Fast path: check for local workspace markers ($tf or .tf directory)
	if hasTFVCDirMarker(dir) {
		return true
	}

	// Slow path: use tf workfold to detect server workspaces
	return tfWorkfoldDetect(dir)
}

// hasTFVCDirMarker checks for $tf or .tf directory markers (local workspaces).
func hasTFVCDirMarker(dir string) bool {
	// Check for $tf directory (TFVC local workspace marker)
	tfPath := filepath.Join(dir, "$tf")
	if info, err := os.Stat(tfPath); err == nil && info.IsDir() {
		return true
	}

	// Check for .tf directory (some TFVC versions use this)
	dotTfPath := filepath.Join(dir, ".tf")
	if info, err := os.Stat(dotTfPath); err == nil && info.IsDir() {
		return true
	}

	// Walk up parent directories
	parent := filepath.Dir(dir)
	for parent != dir {
		tfPath := filepath.Join(parent, "$tf")
		if info, err := os.Stat(tfPath); err == nil && info.IsDir() {
			return true
		}
		dotTfPath := filepath.Join(parent, ".tf")
		if info, err := os.Stat(dotTfPath); err == nil && info.IsDir() {
			return true
		}
		dir = parent
		parent = filepath.Dir(dir)
	}

	return false
}

// tfWorkfoldDetect uses `tf workfold` to check if a directory is mapped in a
// TFVC server workspace. This is needed because server workspaces do not have
// $tf directories on disk.
func tfWorkfoldDetect(dir string) bool {
	tfExe := findTFCommand()
	if tfExe == "" {
		return false
	}

	ctx := context.Background()
	args := []string{"workfold", "/noprompt", dir}
	cmd := exec.CommandContext(ctx, tfExe, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}

	// If tf workfold returns output containing a server path ($/...),
	// this directory is mapped in a TFVC workspace
	output := string(out)
	return strings.Contains(output, "$/") || strings.Contains(output, dir)
}

// findTFCommand locates the tf.exe command on the system.
// It checks PATH first, then common Visual Studio installation paths.
func findTFCommand() string {
	// Check if tf is on PATH
	if path, err := exec.LookPath("tf"); err == nil {
		return path
	}
	if path, err := exec.LookPath("tf.exe"); err == nil {
		return path
	}

	// Check common Visual Studio Team Explorer paths
	vsPaths := []string{
		// VS 2022 (v17)
		`C:\Program Files\Microsoft Visual Studio\2022\Enterprise\Common7\IDE\CommonExtensions\Microsoft\TeamFoundation\Team Explorer\TF.exe`,
		`C:\Program Files\Microsoft Visual Studio\2022\Professional\Common7\IDE\CommonExtensions\Microsoft\TeamFoundation\Team Explorer\TF.exe`,
		`C:\Program Files\Microsoft Visual Studio\2022\Community\Common7\IDE\CommonExtensions\Microsoft\TeamFoundation\Team Explorer\TF.exe`,
		// VS 2019 (v16)
		`C:\Program Files (x86)\Microsoft Visual Studio\2019\Enterprise\Common7\IDE\CommonExtensions\Microsoft\TeamFoundation\Team Explorer\TF.exe`,
		`C:\Program Files (x86)\Microsoft Visual Studio\2019\Professional\Common7\IDE\CommonExtensions\Microsoft\TeamFoundation\Team Explorer\TF.exe`,
		`C:\Program Files (x86)\Microsoft Visual Studio\2019\Community\Common7\IDE\CommonExtensions\Microsoft\TeamFoundation\Team Explorer\TF.exe`,
	}

	// Also try to discover VS installations dynamically
	vsWhere := `C:\Program Files (x86)\Microsoft Visual Studio\Installer\vswhere.exe`
	if info, err := os.Stat(vsWhere); err == nil && info.Mode().IsRegular() {
		ctx := context.Background()
		cmd := exec.CommandContext(ctx, vsWhere, "-latest", "-property", "installationPath")
		if out, err := cmd.Output(); err == nil {
			vsDir := strings.TrimSpace(string(out))
			tfPath := filepath.Join(vsDir, `Common7\IDE\CommonExtensions\Microsoft\TeamFoundation\Team Explorer\TF.exe`)
			if _, err := os.Stat(tfPath); err == nil {
				return tfPath
			}
		}
	}

	for _, p := range vsPaths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	return ""
}
