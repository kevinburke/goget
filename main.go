package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/net/html"
)

var httpsFlag = flag.Bool("https", false, "use HTTPS for git clones instead of SSH")

// Config holds the configuration for a goget operation
type Config struct {
	GOPATH      string
	WorkingDir  string
	ImportPath  string
	HasEllipsis bool
}

// GitCommand represents a git command to execute
type GitCommand struct {
	URL        string
	TargetPath string
	Args       []string
}

// discoverGoImport fetches the go-import meta tag from a custom domain
func discoverGoImport(importPath string) (vcs, repoURL string, err error) {
	// Try HTTPS first
	url := fmt.Sprintf("https://%s?go-get=1", importPath)

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Get(url)
	if err != nil {
		return "", "", fmt.Errorf("failed to fetch %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("got status %d from %s", resp.StatusCode, url)
	}

	return parseGoImportMeta(resp.Body, importPath)
}

// parseGoImportMeta extracts the go-import meta tag from HTML
func parseGoImportMeta(r io.Reader, importPath string) (vcs, repoURL string, err error) {
	tokenizer := html.NewTokenizer(r)

	for {
		tokenType := tokenizer.Next()

		switch tokenType {
		case html.ErrorToken:
			err := tokenizer.Err()
			if err == io.EOF {
				return "", "", fmt.Errorf("no go-import meta tag found")
			}
			return "", "", err

		case html.StartTagToken, html.SelfClosingTagToken:
			token := tokenizer.Token()
			if token.Data != "meta" {
				continue
			}

			var name, content string
			for _, attr := range token.Attr {
				if attr.Key == "name" {
					name = attr.Val
				}
				if attr.Key == "content" {
					content = attr.Val
				}
			}

			if name != "go-import" {
				continue
			}

			// Parse content: "prefix vcs repo-url"
			parts := strings.Fields(content)
			if len(parts) != 3 {
				continue
			}

			prefix, vcsType, repoURL := parts[0], parts[1], parts[2]

			// Check if the prefix matches our import path
			if !strings.HasPrefix(importPath, prefix) {
				continue
			}

			return vcsType, repoURL, nil
		}
	}
}

// shouldUseDiscovery determines if we should try HTTP discovery for this import path
func shouldUseDiscovery(importPath string) bool {
	parts := strings.Split(importPath, "/")
	if len(parts) == 0 {
		return false
	}

	domain := parts[0]

	// Skip discovery for well-known Git hosts - we already know their patterns
	if isCommonGitHost(domain) {
		return false
	}

	return true
}

// getRepositoryURL handles special cases and converts import paths to git URLs
func getRepositoryURL(importPath string, useHTTPS bool) string {
	// For custom domains (not github.com, gitlab.com, etc.), try HTTP discovery first
	if shouldUseDiscovery(importPath) {
		if vcs, repoURL, err := discoverGoImport(importPath); err == nil {
			// We only support git for now
			if vcs == "git" {
				return repoURL
			}
			log.Printf("WARN: discovered VCS type %q is not supported, falling back to heuristics", vcs)
		} else {
			log.Printf("WARN: failed to discover go-import meta tag: %v, falling back to heuristics", err)
		}
	}

	// Handle golang.org/x/* packages
	if repo, ok := strings.CutPrefix(importPath, "golang.org/x/"); ok {
		// Remove any subpackage paths - just get the main repo name
		if idx := strings.Index(repo, "/"); idx != -1 {
			repo = repo[:idx]
		}
		return fmt.Sprintf("https://go.googlesource.com/%s", repo)
	}

	// Handle google.golang.org/* packages
	if repo, ok := strings.CutPrefix(importPath, "google.golang.org/"); ok {
		// Remove any subpackage paths - just get the main repo name
		if idx := strings.Index(repo, "/"); idx != -1 {
			repo = repo[:idx]
		}
		return fmt.Sprintf("https://github.com/googleapis/%s", repo)
	}

	// Handle go.opentelemetry.io/* packages
	if repo, ok := strings.CutPrefix(importPath, "go.opentelemetry.io/"); ok {
		// Remove any subpackage paths - just get the main repo name
		if idx := strings.Index(repo, "/"); idx != -1 {
			repo = repo[:idx]
		}
		return fmt.Sprintf("https://github.com/open-telemetry/%s", repo)
	}

	// Default behavior: construct SSH or HTTPS URL from import path
	parts := strings.Split(importPath, "/")
	if len(parts) == 0 {
		return ""
	}

	domain := parts[0]

	// For common Git hosting services, the repository is typically at
	// domain/user/repo, and anything after that is just a subpackage
	if isCommonGitHost(domain) && len(parts) >= 3 {
		// Take only domain/user/repo for the git URL
		repo := strings.Join(parts[1:3], "/")
		if useHTTPS {
			return fmt.Sprintf("https://%s/%s.git", domain, repo)
		}
		return fmt.Sprintf("git@%s:%s.git", domain, repo)
	}

	// For other domains, use the full path (minus domain)
	if len(parts) > 1 {
		repo := strings.Join(parts[1:], "/")
		if useHTTPS {
			return fmt.Sprintf("https://%s/%s.git", domain, repo)
		}
		return fmt.Sprintf("git@%s:%s.git", domain, repo)
	}

	if useHTTPS {
		return fmt.Sprintf("https://%s.git", domain)
	}
	return fmt.Sprintf("git@%s.git", domain)
}

// isCommonGitHost returns true for well-known Git hosting services
// where repositories follow the user/repo pattern
func isCommonGitHost(domain string) bool {
	commonHosts := []string{
		"github.com",
		"gitlab.com",
		"bitbucket.org",
	}

	for _, host := range commonHosts {
		if domain == host {
			return true
		}
	}
	return false
}

// parseImportPath processes the raw argument and extracts the import path and ellipsis flag
func parseImportPath(arg string) (importPath string, hasEllipsis bool, err error) {
	if arg == "" {
		return "", false, fmt.Errorf("empty import path")
	}

	if strings.HasSuffix(arg, "/...") {
		return strings.TrimSuffix(arg, "/..."), true, nil
	}

	return arg, false, nil
}

// validateImportPath checks if the import path is valid
func validateImportPath(importPath string) error {
	parts := strings.Split(importPath, "/")
	if len(parts) == 0 {
		return fmt.Errorf("no package to retrieve: %v", importPath)
	}

	domain := parts[0]
	if !strings.Contains(domain, ".") {
		return fmt.Errorf("first part of package path should be a domain name, got %v", importPath)
	}

	return nil
}

// resolveConfig takes raw inputs and produces a validated Config
func resolveConfig(arg, gopath, workingDir string) (*Config, error) {
	importPath, hasEllipsis, err := parseImportPath(arg)
	if err != nil {
		return nil, err
	}

	if err := validateImportPath(importPath); err != nil {
		return nil, err
	}

	if gopath == "" {
		return nil, fmt.Errorf("cannot clone without GOPATH set")
	}

	// Handle multiple paths in GOPATH
	if strings.Contains(gopath, ":") {
		gopath = gopath[:strings.Index(gopath, ":")]
	}

	absGopath, err := filepath.Abs(gopath)
	if err != nil {
		return nil, fmt.Errorf("could not get absolute directory for gopath: %v", err)
	}

	return &Config{
		GOPATH:      absGopath,
		WorkingDir:  workingDir,
		ImportPath:  importPath,
		HasEllipsis: hasEllipsis,
	}, nil
}

// buildGitCommand creates the git command from the config
func buildGitCommand(config *Config, useHTTPS bool) (*GitCommand, error) {
	var gitURL string
	var checkoutPath string

	if strings.HasPrefix(config.ImportPath, ".") { // relative path
		if config.WorkingDir == "" {
			return nil, fmt.Errorf("working directory required for relative paths")
		}

		absWD, err := filepath.Abs(config.WorkingDir)
		if err != nil {
			return nil, fmt.Errorf("could not get absolute directory: %v", err)
		}

		rel, err := filepath.Rel(config.GOPATH, absWD)
		if err != nil {
			return nil, fmt.Errorf("could not construct relationship between gopath %q and wd %q: %v", config.GOPATH, absWD, err)
		}

		if !strings.HasPrefix(rel, "src/") {
			return nil, fmt.Errorf("working directory should be contained inside $GOPATH/src, got %q", rel)
		}

		pkgstart := strings.TrimPrefix(rel, "src/")
		fullpkg := filepath.Join(pkgstart, config.ImportPath)
		checkoutPath = config.ImportPath
		gitURL = getRepositoryURL(fullpkg, useHTTPS)
	} else {
		checkoutPath = filepath.Join(config.GOPATH, "src", config.ImportPath)
		gitURL = getRepositoryURL(config.ImportPath, useHTTPS)
	}

	if gitURL == "" {
		return nil, fmt.Errorf("could not determine git URL for %v", config.ImportPath)
	}

	return &GitCommand{
		URL:        gitURL,
		TargetPath: checkoutPath,
		Args:       []string{"clone", gitURL, checkoutPath},
	}, nil
}

// executeGitCommand runs the git command
func executeGitCommand(ctx context.Context, cmd *GitCommand) error {
	gitCmd := exec.CommandContext(ctx, "git", cmd.Args...)
	gitCmd.Stdout = os.Stdout
	gitCmd.Stderr = os.Stderr
	return gitCmd.Run()
}

// runGoGet is the main logic, extracted from main() for testability
func runGoGet(ctx context.Context, arg, gopath, workingDir string, useHTTPS bool) error {
	config, err := resolveConfig(arg, gopath, workingDir)
	if err != nil {
		return err
	}

	if config.HasEllipsis {
		fmt.Printf("Stripping /... suffix, will clone: %s\n", config.ImportPath)
	}

	if strings.Contains(config.GOPATH, ":") {
		log.Printf("WARN: multiple paths in GOPATH; goget only works with first one")
	}

	gitCmd, err := buildGitCommand(config, useHTTPS)
	if err != nil {
		return err
	}

	fmt.Printf("git %s\n", strings.Join(gitCmd.Args, " "))

	if err := executeGitCommand(ctx, gitCmd); err != nil {
		return fmt.Errorf("error running git %v: %v", strings.Join(gitCmd.Args, " "), err)
	}

	if config.HasEllipsis {
		fmt.Printf("Successfully cloned %s (note: /... means this package and all subpackages)\n", config.ImportPath)
	}

	return nil
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	flag.Parse()
	arg := flag.Arg(0)
	if arg == "" {
		log.Fatal("usage: goget <path>")
	}

	gopath := os.Getenv("GOPATH")
	workingDir, err := os.Getwd()
	if err != nil {
		log.Fatalf("could not determine working directory: %v", err)
	}

	if err := runGoGet(ctx, arg, gopath, workingDir, *httpsFlag); err != nil {
		log.Fatal(err)
	}
}
