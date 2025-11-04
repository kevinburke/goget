package main

import (
	"bufio"
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
	"sync"
	"time"

	"golang.org/x/net/html"
	"golang.org/x/sync/errgroup"
)

var httpsFlag = flag.Bool("https", false, "use HTTPS for git clones instead of SSH")
var modFlag = flag.String("mod", "", "path to go.mod file to fetch all direct dependencies")

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

// HTTPClient is an interface for making HTTP requests (for testing)
type HTTPClient interface {
	Get(url string) (*http.Response, error)
}

// discoverGoImport fetches the go-import meta tag from a custom domain
func discoverGoImport(importPath string, client HTTPClient) (vcs, repoURL string, err error) {
	// Try HTTPS first
	url := fmt.Sprintf("https://%s?go-get=1", importPath)

	if client == nil {
		client = &http.Client{
			Timeout: 10 * time.Second,
		}
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

	// Skip discovery for well-known Go-specific domains that have special handling
	wellKnownGoDomains := []string{
		"golang.org",
		"google.golang.org",
		"go.opentelemetry.io",
	}

	for _, knownDomain := range wellKnownGoDomains {
		if domain == knownDomain || strings.HasPrefix(importPath, knownDomain+"/") {
			return false
		}
	}

	return true
}

// getRepositoryURL handles special cases and converts import paths to git URLs
func getRepositoryURL(importPath string, useHTTPS bool) string {
	return getRepositoryURLWithClient(importPath, useHTTPS, nil)
}

// getRepositoryURLWithClient is like getRepositoryURL but accepts an HTTPClient for testing
func getRepositoryURLWithClient(importPath string, useHTTPS bool, client HTTPClient) string {
	// For custom domains (not github.com, gitlab.com, etc.), try HTTP discovery first
	if shouldUseDiscovery(importPath) {
		if vcs, repoURL, err := discoverGoImport(importPath, client); err == nil {
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
		Args:       []string{"clone", "--quiet", gitURL, checkoutPath},
	}, nil
}

// executeGitCommand runs the git command
// Returns (skipped=true, nil) if the repo already exists, (skipped=false, nil) if cloned successfully, or (skipped=false, err) on error
func executeGitCommand(ctx context.Context, cmd *GitCommand) (skipped bool, err error) {
	// Check if a go.mod file exists at the target path
	// This handles monorepos where the .git is at a parent level
	modFile := filepath.Join(cmd.TargetPath, "go.mod")
	if _, err := os.Stat(modFile); err == nil {
		fmt.Printf("Package already exists at %s (go.mod found), skipping clone\n", cmd.TargetPath)
		return true, nil
	}

	// Also check for .git directory at the exact target path (for non-module repos)
	gitDir := filepath.Join(cmd.TargetPath, ".git")
	if _, err := os.Stat(gitDir); err == nil {
		fmt.Printf("Repository already exists at %s, skipping clone\n", cmd.TargetPath)
		return true, nil
	}

	gitCmd := exec.CommandContext(ctx, "git", cmd.Args...)
	gitCmd.Stdout = os.Stdout
	gitCmd.Stderr = os.Stderr
	return false, gitCmd.Run()
}

// parseGoMod parses a go.mod file and returns all dependencies (both direct and indirect)
func parseGoMod(modPath string) ([]string, error) {
	file, err := os.Open(modPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open go.mod file: %w", err)
	}
	defer file.Close()

	var deps []string
	scanner := bufio.NewScanner(file)
	inRequireBlock := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Check if we're entering a require block
		if strings.HasPrefix(line, "require (") {
			inRequireBlock = true
			continue
		}

		// Check if we're leaving a require block
		if inRequireBlock && strings.HasPrefix(line, ")") {
			inRequireBlock = false
			continue
		}

		// Parse require lines (both single-line and multi-line formats)
		var depLine string
		if inRequireBlock {
			depLine = line
		} else if strings.HasPrefix(line, "require ") {
			depLine = strings.TrimPrefix(line, "require ")
		}

		if depLine == "" {
			continue
		}

		// Extract the package path (first field before version)
		fields := strings.Fields(depLine)
		if len(fields) >= 2 {
			deps = append(deps, fields[0])
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading go.mod file: %w", err)
	}

	return deps, nil
}

// DependencyResult holds the result of fetching a single dependency
type DependencyResult struct {
	ImportPath string
	Error      error
	Skipped    bool
}

// runGoGetParallel fetches multiple dependencies in parallel
func runGoGetParallel(ctx context.Context, deps []string, gopath, workingDir string, useHTTPS bool) []DependencyResult {
	results := make([]DependencyResult, len(deps))

	// Use a mutex to ensure git output doesn't get interleaved
	var outputMutex sync.Mutex

	// Use errgroup with concurrency limit to avoid spawning too many goroutines
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(10) // Limit to 10 concurrent git clones

	for i, dep := range deps {
		idx := i
		importPath := dep
		g.Go(func() error {
			// Lock output to prevent interleaving
			outputMutex.Lock()
			fmt.Printf("\n[%d/%d] Fetching %s...\n", idx+1, len(deps), importPath)
			outputMutex.Unlock()

			skipped, err := runGoGet(ctx, importPath, gopath, workingDir, useHTTPS)
			results[idx] = DependencyResult{
				ImportPath: importPath,
				Error:      err,
				Skipped:    skipped,
			}

			outputMutex.Lock()
			if err != nil {
				fmt.Printf("[%d/%d] ERROR: Failed to fetch %s: %v\n", idx+1, len(deps), importPath, err)
			} else if skipped {
				fmt.Printf("[%d/%d] SKIPPED: %s\n", idx+1, len(deps), importPath)
			} else {
				fmt.Printf("[%d/%d] SUCCESS: Fetched %s\n", idx+1, len(deps), importPath)
			}
			outputMutex.Unlock()

			// Don't return error - we want to continue fetching all deps
			return nil
		})
	}

	// Wait for all goroutines to complete
	_ = g.Wait() // We ignore this error since we collect errors in results

	return results
}

// runGoGet is the main logic, extracted from main() for testability
// Returns (skipped=true, nil) if the repo already exists, (skipped=false, nil) if cloned successfully, or (skipped=false, err) on error
func runGoGet(ctx context.Context, arg, gopath, workingDir string, useHTTPS bool) (skipped bool, err error) {
	config, err := resolveConfig(arg, gopath, workingDir)
	if err != nil {
		return false, err
	}

	if config.HasEllipsis {
		fmt.Printf("Stripping /... suffix, will clone: %s\n", config.ImportPath)
	}

	if strings.Contains(config.GOPATH, ":") {
		log.Printf("WARN: multiple paths in GOPATH; goget only works with first one")
	}

	gitCmd, err := buildGitCommand(config, useHTTPS)
	if err != nil {
		return false, err
	}

	fmt.Printf("git %s\n", strings.Join(gitCmd.Args, " "))

	skipped, err = executeGitCommand(ctx, gitCmd)
	if err != nil {
		return false, fmt.Errorf("error running git %v: %v", strings.Join(gitCmd.Args, " "), err)
	}

	if config.HasEllipsis && !skipped {
		fmt.Printf("Successfully cloned %s (note: /... means this package and all subpackages)\n", config.ImportPath)
	}

	return skipped, nil
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	flag.Parse()

	gopath := os.Getenv("GOPATH")
	workingDir, err := os.Getwd()
	if err != nil {
		log.Fatalf("could not determine working directory: %v", err)
	}

	// Handle --mod flag
	if *modFlag != "" {
		fmt.Printf("Parsing dependencies from %s...\n", *modFlag)
		deps, err := parseGoMod(*modFlag)
		if err != nil {
			log.Fatal(err)
		}

		if len(deps) == 0 {
			fmt.Println("No dependencies found in go.mod")
			return
		}

		fmt.Printf("Found %d dependencies (direct and indirect)\n", len(deps))
		results := runGoGetParallel(ctx, deps, gopath, workingDir, *httpsFlag)

		// Print summary
		fmt.Println("\n" + strings.Repeat("=", 60))
		fmt.Println("SUMMARY")
		fmt.Println(strings.Repeat("=", 60))

		successCount := 0
		failureCount := 0
		for _, result := range results {
			if result.Error == nil {
				successCount++
			} else {
				failureCount++
			}
		}

		fmt.Printf("Total: %d | Success: %d | Failed: %d\n", len(results), successCount, failureCount)

		if failureCount > 0 {
			fmt.Println("\nFailed dependencies:")
			for _, result := range results {
				if result.Error != nil {
					fmt.Printf("  - %s: %v\n", result.ImportPath, result.Error)
				}
			}
			os.Exit(1)
		}

		return
	}

	// Original single-package behavior
	arg := flag.Arg(0)
	if arg == "" {
		log.Fatal("usage: goget <path> or goget --mod <path/to/go.mod>")
	}

	if _, err := runGoGet(ctx, arg, gopath, workingDir, *httpsFlag); err != nil {
		log.Fatal(err)
	}
}
