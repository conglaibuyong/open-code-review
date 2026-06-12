package vcs

import (
	"context"
	"fmt"
	"strings"
)

// ShelvesetInfo contains metadata about a TFVC shelveset.
type ShelvesetInfo struct {
	Name      string
	Owner     string
	Comment   string
	FileCount int
}

// ListShelvesets lists shelvesets available in the workspace.
// If owner is empty, lists shelvesets for the current user.
func (t *TFVCProvider) ListShelvesets(ctx context.Context, repoDir string, owner string) ([]ShelvesetInfo, error) {
	args := []string{"shelvesets", "/format:detailed", "/noprompt"}
	if owner != "" {
		args = append(args, "/owner:"+owner)
	}

	out, err := t.runTF(ctx, repoDir, args...)
	if err != nil && out == "" {
		return nil, fmt.Errorf("tf shelvesets failed: %w", err)
	}

	return parseShelvesets(out), nil
}

// GetShelvesetDiff retrieves the diff for a specific shelveset.
func (t *TFVCProvider) GetShelvesetDiff(ctx context.Context, repoDir string, shelvesetName string) (string, error) {
	return t.GetDiff(ctx, DiffOptions{
		Mode:      ModeShelveset,
		RepoDir:   repoDir,
		Shelveset: shelvesetName,
	})
}

// ReadShelvesetFile reads a file from a shelveset.
func (t *TFVCProvider) ReadShelvesetFile(ctx context.Context, repoDir string, path string, shelvesetName string) ([]byte, error) {
	serverPath, err := t.localToServerPath(repoDir, path)
	if err != nil {
		return nil, fmt.Errorf("resolve server path for %s: %w", path, err)
	}

	args := []string{
		"view",
		"/shelveset:" + shelvesetName,
		"/noprompt",
		serverPath,
	}

	out, err := t.runTFBytes(ctx, repoDir, args...)
	if err != nil {
		return nil, fmt.Errorf("tf view from shelveset %s: %w", shelvesetName, err)
	}
	return out, nil
}

// parseShelvesets parses the output of `tf shelvesets /format:detailed`.
func parseShelvesets(output string) []ShelvesetInfo {
	var result []ShelvesetInfo
	var current *ShelvesetInfo

	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "Shelveset:") {
			if current != nil {
				result = append(result, *current)
			}
			current = &ShelvesetInfo{}
			// Parse: "Shelveset: name;owner"
			nameOwner := strings.TrimPrefix(line, "Shelveset:")
			nameOwner = strings.TrimSpace(nameOwner)
			parts := strings.SplitN(nameOwner, ";", 2)
			current.Name = parts[0]
			if len(parts) > 1 {
				current.Owner = parts[1]
			}
		} else if strings.HasPrefix(line, "Comment:") && current != nil {
			current.Comment = strings.TrimSpace(strings.TrimPrefix(line, "Comment:"))
		}
	}

	if current != nil {
		result = append(result, *current)
	}

	return result
}
