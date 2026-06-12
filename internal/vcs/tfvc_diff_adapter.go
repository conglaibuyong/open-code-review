package vcs

import (
	"fmt"
	"regexp"
	"strings"
)

// TFVCDiffAdapter converts TFVC unified diff output to Git-compatible unified diff format.
// TFVC's `tf diff /format:unified` output differs from `git diff` in several ways:
//   - File headers use server paths (e.g., "$/Project/file.cs") instead of "a/file" / "b/file"
//   - Missing "diff --git a/... b/..." header lines
//   - Different binary file markers
//   - Possible differences in hunk headers
type TFVCDiffAdapter struct {
	repoDir string
}

// NewTFVCDiffAdapter creates a new adapter for converting TFVC diffs.
func NewTFVCDiffAdapter(repoDir string) *TFVCDiffAdapter {
	return &TFVCDiffAdapter{repoDir}
}

var (
	// tfvcFileHeader matches TFVC unified diff file headers like:
	// --- $/Project/Branch/file.cs;C12345  or  --- file.cs
	tfvcOldFileRe = regexp.MustCompile(`^--- (.+?)(?:\s*;.*)?$`)
	// +++ $/Project/Branch/file.cs;C12345  or  +++ file.cs
	tfvcNewFileRe = regexp.MustCompile(`^\+\+\+ (.+?)(?:\s*;.*)?$`)
	// TFVC binary file marker
	tfvcBinaryRe = regexp.MustCompile(`^Binary files .* differ$`)
	// TFVC diff section header (different from "diff --git")
	tfvcDiffSectionRe = regexp.MustCompile(`^Index:\s+(.+)$`)
	// TFVC "===" separator
	tfvcEqualsRe = regexp.MustCompile(`^={10,}$`)
	// Hunk header
	hunkHeaderRe = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)
)

// ConvertToGitDiff transforms TFVC unified diff output to Git-compatible format.
// The output is compatible with the existing ParseDiffText() parser in internal/diff.
func (a *TFVCDiffAdapter) ConvertToGitDiff(tfvcDiff string) (string, error) {
	if strings.TrimSpace(tfvcDiff) == "" {
		return "", nil
	}

	lines := strings.Split(tfvcDiff, "\n")
	var result strings.Builder
	var currentFile string
	var inHunk bool

	for i := 0; i < len(lines); i++ {
		line := lines[i]

		// Check for TFVC diff section header: "Index: <path>"
		if m := tfvcDiffSectionRe.FindStringSubmatch(line); m != nil {
			currentFile = a.serverPathToLocal(m[1])
			inHunk = false
			continue
		}

		// Skip TFVC "========" separator lines
		if tfvcEqualsRe.MatchString(line) {
			continue
		}

		// Check for "---" old file header
		if m := tfvcOldFileRe.FindStringSubmatch(line); m != nil {
			if currentFile == "" {
				currentFile = a.serverPathToLocal(m[1])
			}
			// Write Git-compatible header
			result.WriteString(fmt.Sprintf("diff --git a/%s b/%s\n", currentFile, currentFile))
			result.WriteString(fmt.Sprintf("--- a/%s\n", currentFile))
			continue
		}

		// Check for "+++" new file header
		if m := tfvcNewFileRe.FindStringSubmatch(line); m != nil {
			if currentFile == "" {
				currentFile = a.serverPathToLocal(m[1])
			}
			// Check if this is a new file (old was /dev/null)
			result.WriteString(fmt.Sprintf("+++ b/%s\n", currentFile))
			continue
		}

		// Check for binary file markers
		if tfvcBinaryRe.MatchString(line) {
			if currentFile == "" {
				currentFile = "unknown"
			}
			result.WriteString(fmt.Sprintf("diff --git a/%s b/%s\n", currentFile, currentFile))
			result.WriteString("Binary files differ\n")
			inHunk = false
			continue
		}

		// Check for hunk header
		if hunkHeaderRe.MatchString(line) {
			inHunk = true
			result.WriteString(line)
			result.WriteString("\n")
			continue
		}

		// Pass through all other lines (context, added, deleted)
		if inHunk || strings.HasPrefix(line, "+") || strings.HasPrefix(line, "-") || strings.HasPrefix(line, " ") {
			result.WriteString(line)
			result.WriteString("\n")
		}
	}

	return result.String(), nil
}

// serverPathToLocal converts a TFVC server path to a local relative path.
// E.g., "$/Project/Branch/src/file.cs" → "src/file.cs"
func (a *TFVCDiffAdapter) serverPathToLocal(serverPath string) string {
	// Strip the $/ prefix
	path := strings.TrimPrefix(serverPath, "$/")

	// If the path contains a semicolon (version spec), strip it
	if idx := strings.Index(path, ";"); idx >= 0 {
		path = path[:idx]
	}

	// Try to strip the project/branch prefix.
	// This is a best-effort heuristic; in production, you'd use
	// `tf workfold` to determine the exact mapping.
	// For now, we assume the path after the first two segments is the relative path.
	segments := strings.Split(path, "/")
	if len(segments) > 2 {
		// Strip "$/Project/Branch/" prefix, keep the rest
		path = strings.Join(segments[2:], "/")
	}

	// Normalize backslashes to forward slashes
	path = strings.ReplaceAll(path, "\\", "/")

	return path
}
