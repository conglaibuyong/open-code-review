package vcs

import (
	"fmt"
	"regexp"
	"strings"
)

// TFVCDiffAdapter converts TFVC unified diff output to Git-compatible unified diff format.
type TFVCDiffAdapter struct {
	repoDir     string
	serverPath  string
	workspaceID string
}

// NewTFVCDiffAdapter creates a new adapter for converting TFVC diffs.
func NewTFVCDiffAdapter(repoDir string) *TFVCDiffAdapter {
	return &TFVCDiffAdapter{repoDir: repoDir}
}

// SetServerMapping configures the server-to-local path mapping.
func (a *TFVCDiffAdapter) SetServerMapping(serverPath, localBase string) {
	a.serverPath = strings.TrimSuffix(serverPath, "/")
	a.workspaceID = localBase
}

var (
	// Server path pattern: $/ followed by path segments
	serverPathInHeader = regexp.MustCompile(`\$/[^\s;]+`)
	// Version suffix pattern: ;C12345 or ;12345
	versionSuffix = regexp.MustCompile(`;[Cc]?\d+$`)
	// Hunk header: @@ -1,3 +1,4 @@
	hunkHeaderRe = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)
	// Equals separator line (10 or more = signs)
	tfvcEqualsRe = regexp.MustCompile(`^={10,}$`)
	// Binary file marker
	tfvcBinaryRe = regexp.MustCompile(`(?i)^binary files .* differ$`)
)

// ConvertToGitDiff transforms TFVC unified diff output to Git-compatible format.
func (a *TFVCDiffAdapter) ConvertToGitDiff(tfvcDiff string) (string, error) {
	if strings.TrimSpace(tfvcDiff) == "" {
		return "", nil
	}

	// Normalize line endings
	normalized := strings.ReplaceAll(tfvcDiff, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "")

	lines := strings.Split(normalized, "\n")
	var result strings.Builder
	var currentFile string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		// Skip TFVC "========" separator lines
		if tfvcEqualsRe.MatchString(trimmed) {
			continue
		}

		// --- header line
		if strings.HasPrefix(trimmed, "--- ") {
			currentFile = a.extractPathFromOldHeader(trimmed)
			result.WriteString(fmt.Sprintf("diff --git a/%s b/%s\n", currentFile, currentFile))
			result.WriteString(fmt.Sprintf("--- a/%s\n", currentFile))
			continue
		}

		// +++ header line
		if strings.HasPrefix(trimmed, "+++ ") {
			if currentFile == "" {
				currentFile = a.extractPathFromNewHeader(trimmed)
			}
			result.WriteString(fmt.Sprintf("+++ b/%s\n", currentFile))
			continue
		}

		// Hunk header
		if hunkHeaderRe.MatchString(trimmed) {
			if currentFile == "" {
				continue // skip orphan hunks
			}
			result.WriteString(trimmed)
			result.WriteString("\n")
			continue
		}

		// Binary file marker
		if tfvcBinaryRe.MatchString(trimmed) {
			if currentFile == "" {
				currentFile = "unknown"
			}
			result.WriteString(fmt.Sprintf("diff --git a/%s b/%s\n", currentFile, currentFile))
			result.WriteString("Binary files differ\n")
			currentFile = "" // reset for next file
			continue
		}

		// Diff content lines (added, deleted, context)
		if len(trimmed) > 0 && (trimmed[0] == '+' || trimmed[0] == '-' || trimmed[0] == ' ') {
			if currentFile == "" {
				continue // skip orphan lines
			}
			result.WriteString(trimmed)
			result.WriteString("\n")
			continue
		}

		// Everything else (edit/file labels, etc.) — skip and reset currentFile
		// unless we're already in a hunk (context lines start with space)
		currentFile = ""
	}

	return result.String(), nil
}

// extractPathFromOldHeader extracts the file path from a TFVC "---" diff header.
// Format examples:
//   "--- $/DP-Teld/MPM/Main/02-Backend/BBC/TeldBOM_BBC/NuGet.Config;C66981"
//   "--- 服务器: $/DP-Teld/MPM/Main/02-Backend/BBC/TeldBOM_BBC/NuGet.Config;C66981"
// Strategy: find the $/...; pattern which is the server path, convert to local.
func (a *TFVCDiffAdapter) extractPathFromOldHeader(line string) string {
	// Look for server path ($/...) — this is the most reliable indicator
	if m := serverPathInHeader.FindString(line); m != "" {
		// Remove version suffix
		path := versionSuffix.ReplaceAllString(m, "")
		return a.serverPathToLocal(path)
	}

	// Fallback: extract content after "--- " and try to get a path
	content := strings.TrimPrefix(line, "--- ")
	content = strings.TrimSpace(content)
	content = versionSuffix.ReplaceAllString(content, "")
	return a.normalizePath(content)
}

// extractPathFromNewHeader extracts the file path from a TFVC "+++" diff header.
// Format examples:
//   "+++ NuGet.Config"
//   "+++ 本地: NuGet.Config"
//   "+++ Teld.Bom.BBC.Repository\Common\BackUpMasterRepository.cs"
// Strategy: take the last whitespace-separated token that looks like a path.
func (a *TFVCDiffAdapter) extractPathFromNewHeader(line string) string {
	content := strings.TrimPrefix(line, "+++ ")
	content = strings.TrimSpace(content)
	content = versionSuffix.ReplaceAllString(content, "")

	// If content starts with $/, it's a server path
	if strings.HasPrefix(content, "$/") {
		return a.serverPathToLocal(content)
	}

	// The path is typically the last token in the line
	// Split by whitespace and take the last token that looks like a path
	tokens := strings.Fields(content)
	if len(tokens) == 0 {
		return content
	}

	// Take the last token — it's usually the filename/path
	last := tokens[len(tokens)-1]
	return a.normalizePath(last)
}

// serverPathToLocal converts a TFVC server path to a local relative path.
func (a *TFVCDiffAdapter) serverPathToLocal(serverPath string) string {
	path := strings.TrimPrefix(serverPath, "$/")

	if idx := strings.Index(path, ";"); idx >= 0 {
		path = path[:idx]
	}

	if a.serverPath != "" {
		prefix := strings.TrimPrefix(a.serverPath, "$/") + "/"
		if strings.HasPrefix(path, prefix) {
			return strings.ReplaceAll(strings.TrimPrefix(path, prefix), "\\", "/")
		}
	}

	// Fallback: strip first two segments (project/branch)
	segments := strings.Split(path, "/")
	if len(segments) > 2 {
		path = strings.Join(segments[2:], "/")
	}

	return strings.ReplaceAll(path, "\\", "/")
}

// localPathToRelative converts a full local path to a relative path.
func (a *TFVCDiffAdapter) localPathToRelative(localPath string) string {
	path := strings.ReplaceAll(strings.TrimSpace(localPath), "\\", "/")
	repoDir := strings.ReplaceAll(a.repoDir, "\\", "/")

	if strings.HasPrefix(path, repoDir+"/") {
		return strings.TrimPrefix(path, repoDir+"/")
	}
	if strings.HasPrefix(path, repoDir) {
		return strings.TrimPrefix(path, repoDir)
	}
	return path
}

// normalizePath normalizes a path (backslashes → forward slashes, trim).
func (a *TFVCDiffAdapter) normalizePath(path string) string {
	path = strings.TrimSpace(path)
	path = strings.ReplaceAll(path, "\\", "/")
	return path
}