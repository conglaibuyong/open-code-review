package vcs

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// TFVCProvider implements the Provider interface for TFVC (Team Foundation Version Control).
// It uses the `tf` command-line tool to interact with TFVC repositories.
type TFVCProvider struct {
	tfPath string // path to the tf command, defaults to "tf"
}

// NewTFVCProvider creates a TFVCProvider.
// If tfPath is empty, it auto-discovers the tf command location
// by checking PATH and common Visual Studio installation paths.
func NewTFVCProvider(tfPath string) *TFVCProvider {
	if tfPath == "" {
		tfPath = findTFCommand()
	}
	if tfPath == "" {
		tfPath = "tf" // fallback, will fail at runtime with a clear error
	}
	return &TFVCProvider{tfPath: tfPath}
}

func (t *TFVCProvider) Name() Backend { return TFVC }

func (t *TFVCProvider) IgnoreFile() string { return ".tfignore" }

// Detect checks whether the given directory belongs to a TFVC workspace.
// Supports both local workspaces (with $tf directory) and server workspaces
// (detected via `tf workfold`).
func (t *TFVCProvider) Detect(repoDir string) bool {
	return isTFVCWorkspace(repoDir)
}

// ResolveRepoDir validates that the directory is part of a TFVC workspace.
func (t *TFVCProvider) ResolveRepoDir(dir string) (string, error) {
	absPath, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("resolve absolute path: %w", err)
	}

	if !t.Detect(absPath) {
		return "", fmt.Errorf("%s is not a TFVC workspace (checked for $tf directory and tf workfold mapping)", absPath)
	}
	return absPath, nil
}

// GetDiff retrieves diff content from TFVC in unified diff format.
func (t *TFVCProvider) GetDiff(ctx context.Context, opts DiffOptions) (string, error) {
	switch opts.Mode {
	case ModeWorkspace:
		return t.getWorkspaceDiff(ctx, opts)
	case ModeCommit:
		return t.getChangesetDiff(ctx, opts)
	case ModeRange:
		return t.getRangeDiff(ctx, opts)
	case ModeShelveset:
		return t.getShelvesetDiff(ctx, opts)
	default:
		return "", fmt.Errorf("unknown diff mode: %d", opts.Mode)
	}
}

// getWorkspaceDiff gets pending changes diff using `tf diff /recursive /format:unified`.
func (t *TFVCProvider) getWorkspaceDiff(ctx context.Context, opts DiffOptions) (string, error) {
	args := []string{
		"diff",
		"/recursive",
		"/format:unified",
		"/noprompt",
	}

	out, err := t.runTF(ctx, opts.RepoDir, args...)
	if err != nil {
		// tf diff returns non-zero when there are pending changes in some versions
		// but still outputs the diff. Only fail on real errors.
		if out == "" {
			return "", fmt.Errorf("tf diff failed: %w", err)
		}
	}

	// Get server mapping for accurate path conversion
	serverPath, _ := t.getServerPath(opts.RepoDir)

	// Convert TFVC diff format to Git-compatible unified diff format
	adapter := NewTFVCDiffAdapter(opts.RepoDir)
	if serverPath != "" {
		adapter.SetServerMapping(serverPath, opts.RepoDir)
	}
	result, convErr := adapter.ConvertToGitDiff(out)
	if convErr != nil {
		return "", convErr
	}

	// Also get pending adds (new files) that tf diff may not include.
	// These are already in Git-compatible diff format (from formatNewFileDiff),
	// so we append them directly without converting through the adapter.
	adds, err := t.getPendingAdds(ctx, opts.RepoDir)
	if err == nil && adds != "" {
		result += "\n" + adds
	}

	return result, nil
}

// getChangesetDiff gets diff for a specific changeset.
func (t *TFVCProvider) getChangesetDiff(ctx context.Context, opts DiffOptions) (string, error) {
	csID := normalizeChangesetID(opts.Commit)

	args := []string{
		"diff",
		"/version:C" + csID + "~C" + prevChangeset(csID),
		"/recursive",
		"/format:unified",
		"/noprompt",
	}

	out, err := t.runTF(ctx, opts.RepoDir, args...)
	if err != nil && out == "" {
		return "", fmt.Errorf("tf diff for changeset %s failed: %w", csID, err)
	}

	adapter := NewTFVCDiffAdapter(opts.RepoDir)
	return adapter.ConvertToGitDiff(out)
}

// getRangeDiff gets diff between two changesets.
func (t *TFVCProvider) getRangeDiff(ctx context.Context, opts DiffOptions) (string, error) {
	fromID := normalizeChangesetID(opts.From)
	toID := normalizeChangesetID(opts.To)

	args := []string{
		"diff",
		"/version:C" + fromID + "~C" + toID,
		"/recursive",
		"/format:unified",
		"/noprompt",
	}

	out, err := t.runTF(ctx, opts.RepoDir, args...)
	if err != nil && out == "" {
		return "", fmt.Errorf("tf diff range C%s~C%s failed: %w", fromID, toID, err)
	}

	adapter := NewTFVCDiffAdapter(opts.RepoDir)
	return adapter.ConvertToGitDiff(out)
}

// getShelvesetDiff gets diff for a shelveset.
func (t *TFVCProvider) getShelvesetDiff(ctx context.Context, opts DiffOptions) (string, error) {
	args := []string{
		"diff",
		"/shelveset:" + opts.Shelveset,
		"/recursive",
		"/format:unified",
		"/noprompt",
	}

	out, err := t.runTF(ctx, opts.RepoDir, args...)
	if err != nil && out == "" {
		return "", fmt.Errorf("tf diff shelveset %s failed: %w", opts.Shelveset, err)
	}

	adapter := NewTFVCDiffAdapter(opts.RepoDir)
	return adapter.ConvertToGitDiff(out)
}

// ReadFile reads file content from TFVC.
// For workspace mode (ref empty), reads from disk.
// For specific changeset, uses `tf view`.
func (t *TFVCProvider) ReadFile(ctx context.Context, repoDir string, path string, ref string) ([]byte, error) {
	if ref == "" {
		// Workspace mode: read from disk
		fullPath := filepath.Join(repoDir, path)
		content, err := os.ReadFile(fullPath)
		if err != nil {
			return nil, fmt.Errorf("read file %q: %w", path, err)
		}
		return content, nil
	}

	// Use tf view to read a file at a specific version
	serverPath, err := t.localToServerPath(repoDir, path)
	if err != nil {
		return nil, fmt.Errorf("resolve server path for %s: %w", path, err)
	}

	args := []string{
		"view",
		"/version:C" + normalizeChangesetID(ref),
		"/noprompt",
		serverPath,
	}

	out, err := t.runTFBytes(ctx, repoDir, args...)
	if err != nil {
		return nil, fmt.Errorf("tf view %s at C%s: %w", serverPath, ref, err)
	}
	return out, nil
}

// GetCurrentBranch returns the current TFVC workspace/server path info.
func (t *TFVCProvider) GetCurrentBranch(repoDir string) string {
	ctx := context.Background()
	// Use tf workfold to get workspace mapping info
	args := []string{"workfold", "/noprompt"}
	out, err := t.runTF(ctx, repoDir, args...)
	if err != nil {
		return ""
	}

	// Parse the output to find the server path for the current directory
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.Contains(line, ":") && strings.Contains(line, repoDir) {
			// Extract server path (e.g., "$/Project/Branch")
			parts := strings.Fields(line)
			for _, p := range parts {
				if strings.HasPrefix(p, "$/") {
					return p
				}
			}
		}
	}
	return ""
}

// GetCommitMessage returns the changeset comment for the given changeset ID.
func (t *TFVCProvider) GetCommitMessage(repoDir string, ref string) (string, error) {
	ctx := context.Background()
	csID := normalizeChangesetID(ref)

	args := []string{
		"history",
		"/version:C" + csID,
		"/stopafter:1",
		"/format:brief",
		"/noprompt",
	}

	out, err := t.runTF(ctx, repoDir, args...)
	if err != nil {
		return "", fmt.Errorf("tf history for changeset %s failed: %w", csID, err)
	}

	// Parse the brief history output to extract the comment
	return parseChangesetComment(out), nil
}

// getPendingAdds gets the list of pending add operations and constructs
// unified diff entries for new files.
func (t *TFVCProvider) getPendingAdds(ctx context.Context, repoDir string) (string, error) {
	args := []string{
		"status",
		"/recursive",
		"/format:detailed",
		"/noprompt",
	}

	out, err := t.runTF(ctx, repoDir, args...)
	if err != nil && out == "" {
		return "", fmt.Errorf("tf status failed: %w", err)
	}

	var adds []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if !strings.Contains(line, "add") && !strings.Contains(line, "Add") {
			continue
		}
		// Extract local file path from the status output
		localPath := extractLocalPath(line, repoDir)
		if localPath == "" {
			continue
		}
		diff := formatNewFileDiff(localPath, repoDir)
		if diff != "" {
			adds = append(adds, diff)
		}
	}

	return strings.Join(adds, "\n\n"), nil
}

// localToServerPath converts a local relative path to a TFVC server path
// using `tf workfold` mapping information.
func (t *TFVCProvider) localToServerPath(repoDir string, localPath string) (string, error) {
	ctx := context.Background()
	args := []string{"workfold", "/noprompt"}
	out, err := t.runTF(ctx, repoDir, args...)
	if err != nil {
		return "", fmt.Errorf("tf workfold failed: %w", err)
	}

	// Parse workfold output to find the server path mapping
	// Format: "  $/Server/Path: C:\Local\Path"
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		serverPath := strings.TrimSpace(parts[0])
		localMapping := strings.TrimSpace(parts[1])
		if !strings.HasPrefix(serverPath, "$/") {
			continue
		}
		// Check if the file is under this mapping
		if strings.HasPrefix(filepath.Join(repoDir, localPath), localMapping) {
			relFromMapping := strings.TrimPrefix(filepath.Join(repoDir, localPath), localMapping)
			relFromMapping = strings.ReplaceAll(relFromMapping, "\\", "/")
			return serverPath + relFromMapping, nil
		}
	}

	// Fallback: construct server path from relative path
	return "$/" + localPath, nil
}

// runTF executes a tf command and returns the combined output as a string.
// It sets MSYS_NO_PATHCONV=1 to prevent Git Bash from converting /flags to paths.
func (t *TFVCProvider) runTF(ctx context.Context, repoDir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, t.tfPath, args...)
	cmd.Dir = repoDir
	// Prevent Git Bash (MSYS) from converting /flags like /recursive to Windows paths
	cmd.Env = append(os.Environ(), "MSYS_NO_PATHCONV=1")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// runTFBytes executes a tf command and returns stdout only.
func (t *TFVCProvider) runTFBytes(ctx context.Context, repoDir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, t.tfPath, args...)
	cmd.Dir = repoDir
	cmd.Env = append(os.Environ(), "MSYS_NO_PATHCONV=1")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("%w: %s", err, stderr.String())
	}
	return stdout.Bytes(), nil
}

// normalizeChangesetID strips the leading "C" prefix if present.
// TFVC changeset IDs are numeric, e.g., "C12345" or "12345".
func normalizeChangesetID(ref string) string {
	return strings.TrimPrefix(ref, "C")
}

// prevChangeset returns the changeset ID minus one, for diffing against
// the previous version.
func prevChangeset(csID string) string {
	// Parse as integer, subtract 1
	var n int
	fmt.Sscanf(csID, "%d", &n)
	if n > 1 {
		return fmt.Sprintf("%d", n-1)
	}
	return "0"
}

// parseChangesetComment extracts the comment from tf history brief output.
func parseChangesetComment(output string) string {
	lines := strings.Split(output, "\n")
	for i, line := range lines {
		if strings.Contains(line, "Changeset") || strings.Contains(line, "User") {
			// The comment is usually on the next line(s) after the header
			if i+1 < len(lines) {
				return strings.TrimSpace(lines[i+1])
			}
		}
	}
	return strings.TrimSpace(output)
}

// extractLocalPath extracts a local file path from tf status output line.
func extractLocalPath(line string, repoDir string) string {
	// tf status detailed output format varies; try to extract a path
	// that is under repoDir
	fields := strings.Fields(line)
	for _, f := range fields {
		f = strings.Trim(f, "\"")
		if strings.HasPrefix(f, repoDir) || strings.Contains(f, "\\") {
			// Convert to relative path
			rel, err := filepath.Rel(repoDir, f)
			if err == nil {
				return rel
			}
		}
	}
	return ""
}

// getServerPath uses `tf workfold` to determine the server path mapping
// for the given local directory. Returns the server path (e.g., "$/Project/Branch").
func (t *TFVCProvider) getServerPath(repoDir string) (string, error) {
	ctx := context.Background()
	args := []string{"workfold", repoDir}
	out, err := t.runTF(ctx, repoDir, args...)
	if err != nil {
		return "", fmt.Errorf("tf workfold failed: %w", err)
	}

	// Parse the workfold output to find the server path
	// Format varies by locale, e.g.:
	// English: "Workspace: ws-name (Owner)"
	//          "Collection: https://server/tfs/collection"
	//          "$/Project/Branch: D:\local\path"
	// Chinese: "工作区: ws-name (Owner)"
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		// Look for lines containing server paths ($/)
		if idx := strings.Index(line, "$/"); idx >= 0 {
			serverPath := line[idx:]
			// Strip the colon and local path after it
			if colonIdx := strings.Index(serverPath, ":"); colonIdx >= 0 {
				serverPath = serverPath[:colonIdx]
			}
			return strings.TrimSpace(serverPath), nil
		}
	}

	return "", fmt.Errorf("could not determine server path from tf workfold output")
}

// changesetPattern matches changeset references like "C12345" or "12345".
var changesetPattern = regexp.MustCompile(`^C?\d+$`)

// IsChangesetRef returns true if the given string looks like a TFVC changeset reference.
func IsChangesetRef(ref string) bool {
	return changesetPattern.MatchString(ref)
}
