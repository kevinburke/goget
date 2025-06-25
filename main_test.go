package main

import (
	"path/filepath"
	"testing"
)

func TestGetRepositoryURL(t *testing.T) {
	tests := []struct {
		name       string
		importPath string
		expected   string
	}{
		{
			name:       "golang.org/x package",
			importPath: "golang.org/x/sync",
			expected:   "https://go.googlesource.com/sync",
		},
		{
			name:       "golang.org/x package with subpackage",
			importPath: "golang.org/x/crypto/ssh",
			expected:   "https://go.googlesource.com/crypto",
		},
		{
			name:       "github package",
			importPath: "github.com/user/repo",
			expected:   "git@github.com:user/repo.git",
		},
		{
			name:       "github package with subpackage",
			importPath: "github.com/user/repo/subpkg",
			expected:   "git@github.com:user/repo.git",
		},
		{
			name:       "other domain with two parts (full path)",
			importPath: "example.com/foo/bar",
			expected:   "git@example.com:foo/bar.git",
		},
		{
			name:       "github with deep subpackage",
			importPath: "github.com/user/repo/pkg/subpkg",
			expected:   "git@github.com:user/repo.git",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getRepositoryURL(tt.importPath)
			if result != tt.expected {
				t.Errorf("getRepositoryURL(%q) = %q, want %q", tt.importPath, result, tt.expected)
			}
		})
	}
}

func TestParseImportPath(t *testing.T) {
	tests := []struct {
		name             string
		arg              string
		expectedPath     string
		expectedEllipsis bool
		expectError      bool
	}{
		{
			name:             "simple path",
			arg:              "github.com/user/repo",
			expectedPath:     "github.com/user/repo",
			expectedEllipsis: false,
		},
		{
			name:             "path with ellipsis",
			arg:              "github.com/user/repo/...",
			expectedPath:     "github.com/user/repo",
			expectedEllipsis: true,
		},
		{
			name:        "empty path",
			arg:         "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, hasEllipsis, err := parseImportPath(tt.arg)

			if tt.expectError {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if path != tt.expectedPath {
				t.Errorf("path = %q, want %q", path, tt.expectedPath)
			}

			if hasEllipsis != tt.expectedEllipsis {
				t.Errorf("hasEllipsis = %v, want %v", hasEllipsis, tt.expectedEllipsis)
			}
		})
	}
}

func TestValidateImportPath(t *testing.T) {
	tests := []struct {
		name        string
		importPath  string
		expectError bool
	}{
		{
			name:       "valid github path",
			importPath: "github.com/user/repo",
		},
		{
			name:       "valid golang.org path",
			importPath: "golang.org/x/sync",
		},
		{
			name:        "invalid - no domain",
			importPath:  "just-a-name",
			expectError: true,
		},
		{
			name:        "invalid - empty",
			importPath:  "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateImportPath(tt.importPath)

			if tt.expectError && err == nil {
				t.Error("expected error but got none")
			}

			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestBuildGitCommand(t *testing.T) {
	tests := []struct {
		name         string
		config       *Config
		expectedURL  string
		expectedPath string
		expectError  bool
	}{
		{
			name: "absolute path",
			config: &Config{
				GOPATH:     "/home/user/go",
				ImportPath: "github.com/user/repo",
			},
			expectedURL:  "git@github.com:user/repo.git",
			expectedPath: "/home/user/go/src/github.com/user/repo",
		},
		{
			name: "golang.org/x package",
			config: &Config{
				GOPATH:     "/home/user/go",
				ImportPath: "golang.org/x/sync",
			},
			expectedURL:  "https://go.googlesource.com/sync",
			expectedPath: "/home/user/go/src/golang.org/x/sync",
		},
		{
			name: "relative path",
			config: &Config{
				GOPATH:     "/home/user/go",
				WorkingDir: "/home/user/go/src/github.com/user",
				ImportPath: "./repo",
			},
			expectedURL:  "git@github.com:user/repo.git",
			expectedPath: "./repo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, err := buildGitCommand(tt.config)

			if tt.expectError {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if cmd.URL != tt.expectedURL {
				t.Errorf("URL = %q, want %q", cmd.URL, tt.expectedURL)
			}

			if cmd.TargetPath != tt.expectedPath {
				t.Errorf("TargetPath = %q, want %q", cmd.TargetPath, tt.expectedPath)
			}

			expectedArgs := []string{"clone", tt.expectedURL, tt.expectedPath}
			if len(cmd.Args) != len(expectedArgs) {
				t.Errorf("Args length = %d, want %d", len(cmd.Args), len(expectedArgs))
				return
			}

			for i, arg := range cmd.Args {
				if arg != expectedArgs[i] {
					t.Errorf("Args[%d] = %q, want %q", i, arg, expectedArgs[i])
				}
			}
		})
	}
}

func TestResolveConfig(t *testing.T) {
	tests := []struct {
		name             string
		arg              string
		gopath           string
		workingDir       string
		expectedImport   string
		expectedEllipsis bool
		expectError      bool
	}{
		{
			name:             "simple case",
			arg:              "github.com/user/repo",
			gopath:           "/home/user/go",
			workingDir:       "/some/dir",
			expectedImport:   "github.com/user/repo",
			expectedEllipsis: false,
		},
		{
			name:             "with ellipsis",
			arg:              "github.com/user/repo/...",
			gopath:           "/home/user/go",
			workingDir:       "/some/dir",
			expectedImport:   "github.com/user/repo",
			expectedEllipsis: true,
		},
		{
			name:        "empty gopath",
			arg:         "github.com/user/repo",
			gopath:      "",
			workingDir:  "/some/dir",
			expectError: true,
		},
		{
			name:        "invalid import path",
			arg:         "invalid-path",
			gopath:      "/home/user/go",
			workingDir:  "/some/dir",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := resolveConfig(tt.arg, tt.gopath, tt.workingDir)

			if tt.expectError {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if config.ImportPath != tt.expectedImport {
				t.Errorf("ImportPath = %q, want %q", config.ImportPath, tt.expectedImport)
			}

			if config.HasEllipsis != tt.expectedEllipsis {
				t.Errorf("HasEllipsis = %v, want %v", config.HasEllipsis, tt.expectedEllipsis)
			}

			// Check that GOPATH was made absolute
			if !filepath.IsAbs(config.GOPATH) {
				t.Errorf("GOPATH should be absolute, got %q", config.GOPATH)
			}
		})
	}
}
