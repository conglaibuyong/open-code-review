package vcs

import (
	"testing"
)

func TestGitProvider_Name(t *testing.T) {
	p := NewGitProvider(0)
	if p.Name() != Git {
		t.Errorf("Name() = %v, want %v", p.Name(), Git)
	}
}

func TestGitProvider_IgnoreFile(t *testing.T) {
	p := NewGitProvider(0)
	if p.IgnoreFile() != ".gitignore" {
		t.Errorf("IgnoreFile() = %v, want .gitignore", p.IgnoreFile())
	}
}

func TestTFVCProvider_Name(t *testing.T) {
	p := NewTFVCProvider("")
	if p.Name() != TFVC {
		t.Errorf("Name() = %v, want %v", p.Name(), TFVC)
	}
}

func TestTFVCProvider_IgnoreFile(t *testing.T) {
	p := NewTFVCProvider("")
	if p.IgnoreFile() != ".tfignore" {
		t.Errorf("IgnoreFile() = %v, want .tfignore", p.IgnoreFile())
	}
}

func TestNormalizeChangesetID(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"C12345", "12345"},
		{"12345", "12345"},
		{"C1", "1"},
		{"1", "1"},
	}
	for _, tt := range tests {
		got := normalizeChangesetID(tt.input)
		if got != tt.expected {
			t.Errorf("normalizeChangesetID(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestPrevChangeset(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"12345", "12344"},
		{"1", "0"},
		{"2", "1"},
	}
	for _, tt := range tests {
		got := prevChangeset(tt.input)
		if got != tt.expected {
			t.Errorf("prevChangeset(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestIsChangesetRef(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"C12345", true},
		{"12345", true},
		{"abc123", false},
		{"main", false},
		{"C0", true},
		{"", false},
	}
	for _, tt := range tests {
		got := IsChangesetRef(tt.input)
		if got != tt.want {
			t.Errorf("IsChangesetRef(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestTFVCDiffAdapter_ServerPathToLocal(t *testing.T) {
	adapter := NewTFVCDiffAdapter("/repo")
	tests := []struct {
		input    string
		expected string
	}{
		{"$/Project/Branch/src/file.cs", "src/file.cs"},
		{"$/MyApp/Main/utils/helper.go", "utils/helper.go"},
		{"$/A/B/file.txt", "file.txt"},
		{"src/file.cs;C12345", "src/file.cs"},
		{"file.cs", "file.cs"},
	}
	for _, tt := range tests {
		got := adapter.serverPathToLocal(tt.input)
		if got != tt.expected {
			t.Errorf("serverPathToLocal(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestTFVCDiffAdapter_ConvertToGitDiff(t *testing.T) {
	adapter := NewTFVCDiffAdapter("/repo")
	tfvcOutput := `Index: $/Project/Branch/src/main.go
==========
--- $/Project/Branch/src/main.go;C100
+++ $/Project/Branch/src/main.go;C101
@@ -1,5 +1,5 @@
 package main

-func old() {
+func new() {
 }
`

	result, err := adapter.ConvertToGitDiff(tfvcOutput)
	if err != nil {
		t.Fatalf("ConvertToGitDiff error: %v", err)
	}

	// Should contain git-compatible diff headers
	if !contains(result, "diff --git a/src/main.go b/src/main.go") {
		t.Errorf("missing diff --git header in output:\n%s", result)
	}
	if !contains(result, "--- a/src/main.go") {
		t.Errorf("missing --- header in output:\n%s", result)
	}
	if !contains(result, "+++ b/src/main.go") {
		t.Errorf("missing +++ header in output:\n%s", result)
	}
	if !contains(result, "-func old() {") {
		t.Errorf("missing deleted line in output:\n%s", result)
	}
	if !contains(result, "+func new() {") {
		t.Errorf("missing added line in output:\n%s", result)
	}
}

func TestTFVCDiffAdapter_EmptyInput(t *testing.T) {
	adapter := NewTFVCDiffAdapter("/repo")
	result, err := adapter.ConvertToGitDiff("")
	if err != nil {
		t.Fatalf("ConvertToGitDiff error: %v", err)
	}
	if result != "" {
		t.Errorf("expected empty output for empty input, got: %q", result)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
