// Package vcs provides a version control system abstraction layer.
// It defines the Provider interface that each VCS backend (Git, TFVC) must implement,
// enabling the code review tool to work with different version control systems.
package vcs

import (
	"context"
)

// Backend represents a version control system type.
type Backend string

const (
	Git  Backend = "git"
	TFVC Backend = "tfvc"
)

// DiffMode defines how the diff is retrieved.
type DiffMode int

const (
	ModeWorkspace DiffMode = iota // current workspace (pending/staged changes)
	ModeCommit                    // single commit/changeset vs its parent
	ModeRange                     // range between two refs
	ModeShelveset                 // TFVC shelveset (TFVC-specific)
)

// DiffOptions holds parameters for retrieving diffs.
type DiffOptions struct {
	Mode    DiffMode
	RepoDir string
	From    string // range mode: source ref (branch/changeset)
	To      string // range mode: target ref (branch/changeset)
	Commit  string // commit mode: single commit hash or changeset ID
	// Shelveset is the TFVC shelveset name (TFVC-specific, ignored by Git).
	Shelveset string
	// Context is the number of context lines around each change.
	Context int
}

// Provider is the abstraction for version control system operations.
// Each VCS backend (Git, TFVC) must implement this interface.
type Provider interface {
	// Name returns the backend type identifier (e.g., "git", "tfvc").
	Name() Backend

	// Detect checks whether the given directory belongs to this VCS repository.
	Detect(repoDir string) bool

	// GetDiff retrieves diff content in unified diff format.
	GetDiff(ctx context.Context, opts DiffOptions) (string, error)

	// ReadFile reads the content of a file at the given path.
	// In workspace mode, ref is empty and the file is read from disk.
	// In commit/range mode, ref is the VCS-specific reference to read from.
	ReadFile(ctx context.Context, repoDir string, path string, ref string) ([]byte, error)

	// ResolveRepoDir validates and resolves the repository directory.
	ResolveRepoDir(dir string) (string, error)

	// GetCurrentBranch returns the current branch or workspace info.
	GetCurrentBranch(repoDir string) string

	// GetCommitMessage returns the commit/changeset message for the given ref.
	GetCommitMessage(repoDir string, ref string) (string, error)

	// IgnoreFile returns the name of the ignore file used by this VCS
	// (e.g., ".gitignore" for Git, ".tfignore" for TFVC).
	IgnoreFile() string
}
