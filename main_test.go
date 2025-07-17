package main

import (
	"path/filepath"
	"testing"
)

func TestGetRepositoryURL(t *testing.T) {
	tests := []struct {
		name       string
		importPath string
		useHTTPS   bool
		expected   string
	}{
		{
			name:       "golang.org/x package (SSH)",
			importPath: "golang.org/x/sync",
			useHTTPS:   false,
			expected:   "https://go.googlesource.com/sync",
		},
		{
			name:       "golang.org/x package (HTTPS)",
			importPath: "golang.org/x/sync",
			useHTTPS:   true,
			expected:   "https://go.googlesource.com/sync",
		},
		{
			name:       "golang.org/x package with subpackage (SSH)",
			importPath: "golang.org/x/crypto/ssh",
			useHTTPS:   false,
			expected:   "https://go.googlesource.com/crypto",
		},
		{
			name:       "golang.org/x package with subpackage (HTTPS)",
			importPath: "golang.org/x/crypto/ssh",
			useHTTPS:   true,
			expected:   "https://go.googlesource.com/crypto",
		},
		{
			name:       "github package (SSH)",
			importPath: "github.com/user/repo",
			useHTTPS:   false,
			expected:   "git@github.com:user/repo.git",
		},
		{
			name:       "github package (HTTPS)",
			importPath: "github.com/user/repo",
			useHTTPS:   true,
			expected:   "https://github.com/user/repo.git",
		},
		{
			name:       "github package with subpackage (SSH)",
			importPath: "github.com/user/repo/subpkg",
			useHTTPS:   false,
			expected:   "git@github.com:user/repo.git",
		},
		{
			name:       "github package with subpackage (HTTPS)",
			importPath: "github.com/user/repo/subpkg",
			useHTTPS:   true,
			expected:   "https://github.com/user/repo.git",
		},
		{
			name:       "other domain with two parts (SSH)",
			importPath: "example.com/foo/bar",
			useHTTPS:   false,
			expected:   "git@example.com:foo/bar.git",
		},
		{
			name:       "other domain with two parts (HTTPS)",
			importPath: "example.com/foo/bar",
			useHTTPS:   true,
			expected:   "https://example.com/foo/bar.git",
		},
		{
			name:       "github with deep subpackage (SSH)",
			importPath: "github.com/user/repo/pkg/subpkg",
			useHTTPS:   false,
			expected:   "git@github.com:user/repo.git",
		},
		{
			name:       "github with deep subpackage (HTTPS)",
			importPath: "github.com/user/repo/pkg/subpkg",
			useHTTPS:   true,
			expected:   "https://github.com/user/repo.git",
		},
		{
			name:       "gitlab package (SSH)",
			importPath: "gitlab.com/user/repo",
			useHTTPS:   false,
			expected:   "git@gitlab.com:user/repo.git",
		},
		{
			name:       "gitlab package (HTTPS)",
			importPath: "gitlab.com/user/repo",
			useHTTPS:   true,
			expected:   "https://gitlab.com/user/repo.git",
		},
		{
			name:       "bitbucket package (SSH)",
			importPath: "bitbucket.org/user/repo",
			useHTTPS:   false,
			expected:   "git@bitbucket.org:user/repo.git",
		},
		{
			name:       "bitbucket package (HTTPS)",
			importPath: "bitbucket.org/user/repo",
			useHTTPS:   true,
			expected:   "https://bitbucket.org/user/repo.git",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getRepositoryURL(tt.importPath, tt.useHTTPS)
			if result != tt.expected {
				t.Errorf("getRepositoryURL(%q, %v) = %q, want %q", tt.importPath, tt.useHTTPS, result, tt.expected)
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
		useHTTPS     bool
		expectedURL  string
		expectedPath string
		expectError  bool
	}{
		{
			name: "absolute path (SSH)",
			config: &Config{
				GOPATH:     "/home/user/go",
				ImportPath: "github.com/user/repo",
			},
			useHTTPS:     false,
			expectedURL:  "git@github.com:user/repo.git",
			expectedPath: "/home/user/go/src/github.com/user/repo",
		},
		{
			name: "absolute path (HTTPS)",
			config: &Config{
				GOPATH:     "/home/user/go",
				ImportPath: "github.com/user/repo",
			},
			useHTTPS:     true,
			expectedURL:  "https://github.com/user/repo.git",
			expectedPath: "/home/user/go/src/github.com/user/repo",
		},
		{
			name: "golang.org/x package (SSH)",
			config: &Config{
				GOPATH:     "/home/user/go",
				ImportPath: "golang.org/x/sync",
			},
			useHTTPS:     false,
			expectedURL:  "https://go.googlesource.com/sync",
			expectedPath: "/home/user/go/src/golang.org/x/sync",
		},
		{
			name: "golang.org/x package (HTTPS)",
			config: &Config{
				GOPATH:     "/home/user/go",
				ImportPath: "golang.org/x/sync",
			},
			useHTTPS:     true,
			expectedURL:  "https://go.googlesource.com/sync",
			expectedPath: "/home/user/go/src/golang.org/x/sync",
		},
		{
			name: "relative path (SSH)",
			config: &Config{
				GOPATH:     "/home/user/go",
				WorkingDir: "/home/user/go/src/github.com/user",
				ImportPath: "./repo",
			},
			useHTTPS:     false,
			expectedURL:  "git@github.com:user/repo.git",
			expectedPath: "./repo",
		},
		{
			name: "relative path (HTTPS)",
			config: &Config{
				GOPATH:     "/home/user/go",
				WorkingDir: "/home/user/go/src/github.com/user",
				ImportPath: "./repo",
			},
			useHTTPS:     true,
			expectedURL:  "https://github.com/user/repo.git",
			expectedPath: "./repo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, err := buildGitCommand(tt.config, tt.useHTTPS)

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
