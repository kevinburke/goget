package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

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

// getRepositoryURL handles special cases and converts import paths to git URLs
func getRepositoryURL(importPath string) string {
	// Handle golang.org/x/* packages
	if strings.HasPrefix(importPath, "golang.org/x/") {
		repo := strings.TrimPrefix(importPath, "golang.org/x/")
		// Remove any subpackage paths - just get the main repo name
		if idx := strings.Index(repo, "/"); idx != -1 {
			repo = repo[:idx]
		}
		return fmt.Sprintf("https://go.googlesource.com/%s", repo)
	}

	// Handle other special cases as needed
	// You can add more special cases here, for example:
	// if strings.HasPrefix(importPath, "go.uber.org/") {
	//     // Handle uber's vanity URLs
	// }

	// Default behavior: construct SSH URL from import path
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
		return fmt.Sprintf("git@%s:%s.git", domain, repo)
	}

	// For other domains, use the full path (minus domain)
	if len(parts) > 1 {
		repo := strings.Join(parts[1:], "/")
		return fmt.Sprintf("git@%s:%s.git", domain, repo)
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
func buildGitCommand(config *Config) (*GitCommand, error) {
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
		gitURL = getRepositoryURL(fullpkg)
	} else {
		checkoutPath = filepath.Join(config.GOPATH, "src", config.ImportPath)
		gitURL = getRepositoryURL(config.ImportPath)
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
func runGoGet(ctx context.Context, arg, gopath, workingDir string) error {
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

	gitCmd, err := buildGitCommand(config)
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

	if err := runGoGet(ctx, arg, gopath, workingDir); err != nil {
		log.Fatal(err)
	}
}
