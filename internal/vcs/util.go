package vcs

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// formatNewFileDiff constructs a unified diff entry for a newly added file
// by reading its content from disk. This is used for untracked files in Git
// and pending adds in TFVC.
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
