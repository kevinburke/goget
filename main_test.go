package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

// mockHTTPClient is a mock implementation of HTTPClient for testing
type mockHTTPClient struct {
	response *http.Response
	err      error
}

func (m *mockHTTPClient) Get(url string) (*http.Response, error) {
	return m.response, m.err
}

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
			// Use a mock client that always fails to ensure no live HTTP calls
			mockClient := &mockHTTPClient{
				err: io.EOF,
			}
			result := getRepositoryURLWithClient(tt.importPath, tt.useHTTPS, mockClient)
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

			expectedArgs := []string{"clone", "--quiet", tt.expectedURL, tt.expectedPath}
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

func TestShouldUseDiscovery(t *testing.T) {
	tests := []struct {
		name       string
		importPath string
		expected   bool
	}{
		{
			name:       "github.com should not use discovery",
			importPath: "github.com/user/repo",
			expected:   false,
		},
		{
			name:       "gitlab.com should not use discovery",
			importPath: "gitlab.com/user/repo",
			expected:   false,
		},
		{
			name:       "bitbucket.org should not use discovery",
			importPath: "bitbucket.org/user/repo",
			expected:   false,
		},
		{
			name:       "golang.org/x should not use discovery",
			importPath: "golang.org/x/sync",
			expected:   false,
		},
		{
			name:       "google.golang.org should not use discovery",
			importPath: "google.golang.org/protobuf",
			expected:   false,
		},
		{
			name:       "go.opentelemetry.io should not use discovery",
			importPath: "go.opentelemetry.io/otel",
			expected:   false,
		},
		{
			name:       "custom domain should use discovery",
			importPath: "example.com/user/repo",
			expected:   true,
		},
		{
			name:       "another custom domain should use discovery",
			importPath: "mycompany.dev/internal/tools",
			expected:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shouldUseDiscovery(tt.importPath)
			if result != tt.expected {
				t.Errorf("shouldUseDiscovery(%q) = %v, want %v", tt.importPath, result, tt.expected)
			}
		})
	}
}

func TestParseGoImportMeta(t *testing.T) {
	tests := []struct {
		name        string
		html        string
		importPath  string
		expectedVCS string
		expectedURL string
		expectError bool
	}{
		{
			name: "valid go-import meta tag",
			html: `<!DOCTYPE html>
<html>
<head>
	<meta name="go-import" content="google.golang.org/protobuf git https://github.com/protocolbuffers/protobuf-go">
</head>
</html>`,
			importPath:  "google.golang.org/protobuf",
			expectedVCS: "git",
			expectedURL: "https://github.com/protocolbuffers/protobuf-go",
		},
		{
			name: "meta tag with subpackage",
			html: `<!DOCTYPE html>
<html>
<head>
	<meta name="go-import" content="example.com/pkg git https://github.com/example/pkg">
</head>
</html>`,
			importPath:  "example.com/pkg/subpkg",
			expectedVCS: "git",
			expectedURL: "https://github.com/example/pkg",
		},
		{
			name: "multiple meta tags, only one is go-import",
			html: `<!DOCTYPE html>
<html>
<head>
	<meta name="description" content="A Go package">
	<meta name="go-import" content="example.com/pkg git https://github.com/example/pkg">
	<meta name="keywords" content="go, golang">
</head>
</html>`,
			importPath:  "example.com/pkg",
			expectedVCS: "git",
			expectedURL: "https://github.com/example/pkg",
		},
		{
			name: "no go-import meta tag",
			html: `<!DOCTYPE html>
<html>
<head>
	<meta name="description" content="A Go package">
</head>
</html>`,
			importPath:  "example.com/pkg",
			expectError: true,
		},
		{
			name: "malformed content (only 2 parts)",
			html: `<!DOCTYPE html>
<html>
<head>
	<meta name="go-import" content="example.com/pkg git">
</head>
</html>`,
			importPath:  "example.com/pkg",
			expectError: true,
		},
		{
			name: "prefix mismatch",
			html: `<!DOCTYPE html>
<html>
<head>
	<meta name="go-import" content="other.com/pkg git https://github.com/other/pkg">
</head>
</html>`,
			importPath:  "example.com/pkg",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vcs, url, err := parseGoImportMeta(strings.NewReader(tt.html), tt.importPath)

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

			if vcs != tt.expectedVCS {
				t.Errorf("VCS = %q, want %q", vcs, tt.expectedVCS)
			}

			if url != tt.expectedURL {
				t.Errorf("URL = %q, want %q", url, tt.expectedURL)
			}
		})
	}
}

func TestDiscoverGoImport(t *testing.T) {
	tests := []struct {
		name           string
		importPath     string
		responseBody   string
		responseStatus int
		responseError  error
		expectedVCS    string
		expectedURL    string
		expectError    bool
	}{
		{
			name:       "successful discovery",
			importPath: "example.com/pkg",
			responseBody: `<!DOCTYPE html>
<html>
<head>
	<meta name="go-import" content="example.com/pkg git https://github.com/example/pkg">
</head>
</html>`,
			responseStatus: http.StatusOK,
			expectedVCS:    "git",
			expectedURL:    "https://github.com/example/pkg",
		},
		{
			name:       "discovery with subpackage",
			importPath: "example.com/pkg/subpkg",
			responseBody: `<!DOCTYPE html>
<html>
<head>
	<meta name="go-import" content="example.com/pkg git https://github.com/example/pkg">
</head>
</html>`,
			responseStatus: http.StatusOK,
			expectedVCS:    "git",
			expectedURL:    "https://github.com/example/pkg",
		},
		{
			name:           "HTTP 404 error",
			importPath:     "example.com/pkg",
			responseBody:   "Not Found",
			responseStatus: http.StatusNotFound,
			expectError:    true,
		},
		{
			name:          "network error",
			importPath:    "example.com/pkg",
			responseError: io.EOF,
			expectError:   true,
		},
		{
			name:       "no meta tag found",
			importPath: "example.com/pkg",
			responseBody: `<!DOCTYPE html>
<html>
<head>
	<title>No meta tag here</title>
</head>
</html>`,
			responseStatus: http.StatusOK,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var client HTTPClient

			if tt.responseError != nil {
				// Mock a network error
				client = &mockHTTPClient{
					err: tt.responseError,
				}
			} else {
				// Mock a successful response
				client = &mockHTTPClient{
					response: &http.Response{
						StatusCode: tt.responseStatus,
						Body:       io.NopCloser(strings.NewReader(tt.responseBody)),
					},
				}
			}

			vcs, url, err := discoverGoImport(tt.importPath, client)

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

			if vcs != tt.expectedVCS {
				t.Errorf("VCS = %q, want %q", vcs, tt.expectedVCS)
			}

			if url != tt.expectedURL {
				t.Errorf("URL = %q, want %q", url, tt.expectedURL)
			}
		})
	}
}

func TestGetRepositoryURLWithClient(t *testing.T) {
	tests := []struct {
		name         string
		importPath   string
		useHTTPS     bool
		mockResponse string
		mockStatus   int
		expectedURL  string
	}{
		{
			name:       "custom domain with successful discovery",
			importPath: "example.com/pkg",
			useHTTPS:   false,
			mockResponse: `<!DOCTYPE html>
<html>
<head>
	<meta name="go-import" content="example.com/pkg git https://github.com/example/customrepo">
</head>
</html>`,
			mockStatus:  http.StatusOK,
			expectedURL: "https://github.com/example/customrepo",
		},
		{
			name:       "custom domain with failed discovery falls back to heuristics (SSH)",
			importPath: "example.com/user/repo",
			useHTTPS:   false,
			mockResponse: `<!DOCTYPE html>
<html>
<head>
	<title>No meta tag</title>
</head>
</html>`,
			mockStatus:  http.StatusOK,
			expectedURL: "git@example.com:user/repo.git",
		},
		{
			name:       "custom domain with failed discovery falls back to heuristics (HTTPS)",
			importPath: "example.com/user/repo",
			useHTTPS:   true,
			mockResponse: `<!DOCTYPE html>
<html>
<head>
	<title>No meta tag</title>
</head>
</html>`,
			mockStatus:  http.StatusOK,
			expectedURL: "https://example.com/user/repo.git",
		},
		{
			name:        "github.com doesn't use discovery (SSH)",
			importPath:  "github.com/user/repo",
			useHTTPS:    false,
			expectedURL: "git@github.com:user/repo.git",
		},
		{
			name:        "github.com doesn't use discovery (HTTPS)",
			importPath:  "github.com/user/repo",
			useHTTPS:    true,
			expectedURL: "https://github.com/user/repo.git",
		},
		{
			name:        "golang.org/x doesn't use discovery",
			importPath:  "golang.org/x/sync",
			useHTTPS:    false,
			expectedURL: "https://go.googlesource.com/sync",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var client HTTPClient
			if tt.mockResponse != "" {
				client = &mockHTTPClient{
					response: &http.Response{
						StatusCode: tt.mockStatus,
						Body:       io.NopCloser(strings.NewReader(tt.mockResponse)),
					},
				}
			}

			result := getRepositoryURLWithClient(tt.importPath, tt.useHTTPS, client)
			if result != tt.expectedURL {
				t.Errorf("getRepositoryURLWithClient(%q, %v) = %q, want %q", tt.importPath, tt.useHTTPS, result, tt.expectedURL)
			}
		})
	}
}

func TestDiscoverGoImportWithHTTPTest(t *testing.T) {
	// Test using httptest.Server for more realistic HTTP testing
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify the URL query parameter
		if r.URL.Query().Get("go-get") != "1" {
			t.Errorf("expected go-get=1 query parameter, got %v", r.URL.RawQuery)
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<!DOCTYPE html>
<html>
<head>
	<meta name="go-import" content="example.com/pkg git https://github.com/example/pkg">
</head>
</html>`))
	}))
	defer server.Close()

	// For this test, we would need to modify the discoverGoImport function
	// to accept a base URL parameter, or we can test the integration differently.
	// Since we're using a mock client, the httptest approach is less necessary.
	// This test demonstrates the pattern if we wanted to use httptest in the future.
	t.Skip("This test is for demonstration - we're using mock clients instead")
}
