package vcs

import (
	"strings"
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
	adapter.SetServerMapping("$/DP-Teld/MPM/Main/02-Backend/BBC/TeldBOM_BBC", "/repo")
	tests := []struct {
		input    string
		expected string
	}{
		{"$/DP-Teld/MPM/Main/02-Backend/BBC/TeldBOM_BBC/NuGet.Config", "NuGet.Config"},
		{"$/DP-Teld/MPM/Main/02-Backend/BBC/TeldBOM_BBC/Teld.Bom.BBC.Repository/Common/BackUpMasterRepository.cs", "Teld.Bom.BBC.Repository/Common/BackUpMasterRepository.cs"},
	}
	for _, tt := range tests {
		got := adapter.serverPathToLocal(tt.input)
		if got != tt.expected {
			t.Errorf("serverPathToLocal(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestTFVCDiffAdapter_ExtractPathFromOldHeader(t *testing.T) {
	adapter := NewTFVCDiffAdapter("/repo")
	adapter.SetServerMapping("$/DP-Teld/MPM/Main/02-Backend/BBC/TeldBOM_BBC", "/repo/DCCS/DP-Teld/MPM/Main/02-Backend/BBC/TeldBOM_BBC")
	tests := []struct {
		input    string
		expected string
	}{
		{"--- $/DP-Teld/MPM/Main/02-Backend/BBC/TeldBOM_BBC/NuGet.Config;C66981", "NuGet.Config"},
		{"--- $/DP-Teld/MPM/Main/02-Backend/BBC/TeldBOM_BBC/Teld.Bom.BBC.Repository/Common/BackUpMasterRepository.cs;C98332", "Teld.Bom.BBC.Repository/Common/BackUpMasterRepository.cs"},
	}
	for _, tt := range tests {
		got := adapter.extractPathFromOldHeader(tt.input)
		if got != tt.expected {
			t.Errorf("extractPathFromOldHeader(%q)\n= %q\nwant %q", tt.input, got, tt.expected)
		}
	}
}

func TestTFVCDiffAdapter_ExtractPathFromNewHeader(t *testing.T) {
	adapter := NewTFVCDiffAdapter("/repo")
	tests := []struct {
		input    string
		expected string
	}{
		{"+++ NuGet.Config", "NuGet.Config"},
		{"+++ Teld.Bom.BBC.Repository\\Common\\BackUpMasterRepository.cs", "Teld.Bom.BBC.Repository/Common/BackUpMasterRepository.cs"},
	}
	for _, tt := range tests {
		got := adapter.extractPathFromNewHeader(tt.input)
		if got != tt.expected {
			t.Errorf("extractPathFromNewHeader(%q)\n= %q\nwant %q", tt.input, got, tt.expected)
		}
		_ = strings.Contains(got, tt.expected) // suppress unused import warning
	}
}

func TestTFVCDiffAdapter_ConvertToGitDiff(t *testing.T) {
	adapter := NewTFVCDiffAdapter("/repo")
	adapter.SetServerMapping("$/DP-Teld/MPM/Main/02-Backend/BBC/TeldBOM_BBC", "/repo")
	tfvcOutput := `--- $/DP-Teld/MPM/Main/02-Backend/BBC/TeldBOM_BBC/NuGet.Config;C66981
+++ NuGet.Config
@@ -4,13 +4,14 @@
   <config>
-  </config>
+  </config>
 }
`

	result, err := adapter.ConvertToGitDiff(tfvcOutput)
	if err != nil {
		t.Fatalf("ConvertToGitDiff error: %v", err)
	}

	if !strings.Contains(result, "diff --git a/NuGet.Config b/NuGet.Config") {
		t.Errorf("missing diff --git header in output:\n%s", result)
	}
	if !strings.Contains(result, "--- a/NuGet.Config") {
		t.Errorf("missing --- header in output:\n%s", result)
	}
	if !strings.Contains(result, "+++ b/NuGet.Config") {
		t.Errorf("missing +++ header in output:\n%s", result)
	}
}

func TestTFVCDiffAdapter_ConvertWithCRLF(t *testing.T) {
	adapter := NewTFVCDiffAdapter("/repo")
	adapter.SetServerMapping("$/DP-Teld/MPM/Main/02-Backend/BBC/TeldBOM_BBC", "/repo")
	tfvcOutput := "--- $/DP-Teld/MPM/Main/02-Backend/BBC/TeldBOM_BBC/NuGet.Config;C66981\r\n+++ NuGet.Config\r\n@@ -4,13 +4,14 @@\r\n-  </config>\r\n+  </config>\r\n"

	result, err := adapter.ConvertToGitDiff(tfvcOutput)
	if err != nil {
		t.Fatalf("ConvertToGitDiff error: %v", err)
	}
	if !strings.Contains(result, "diff --git a/NuGet.Config b/NuGet.Config") {
		t.Errorf("missing header with CRLF input:\n%s", result)
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