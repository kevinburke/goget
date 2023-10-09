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

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	flag.Parse()
	arg := flag.Arg(0)
	if arg == "" {
		log.Fatal("usage: goget <path>")
	}
	gopath := os.Getenv("GOPATH")
	if gopath == "" {
		log.Fatal("cannot clone without GOPATH set")
	}
	if strings.Contains(gopath, ":") {
		log.Printf("WARN: multiple paths in GOPATH; goget only works with first one (%q)", gopath)
		gopath = gopath[:strings.Index(gopath, ":")]
	}
	var err error
	gopath, err = filepath.Abs(gopath)
	if err != nil {
		log.Fatal("could not get absolute directory for gopath: %v", err)
	}
	if strings.HasPrefix(arg, ".") { // relative path
		wd, err := os.Getwd()
		if err != nil {
			log.Fatal("could not determine working directory: %v", err)
		}
		wd, err = filepath.Abs(wd)
		if err != nil {
			log.Fatal("could not get absolute directory: %v", err)
		}
		rel, err := filepath.Rel(gopath, wd)
		if err != nil {
			log.Fatal("could not construct relationship between gopath %q and wd %q: %v", gopath, wd, err)
		}
		if !strings.HasPrefix(rel, "src/") {
			log.Fatalf("working directory should be contained inside $GOPATH/src, got %q", rel)
		}
		pkgstart := strings.TrimPrefix(rel, "src/")
		fullpkg := filepath.Join(pkgstart, arg)
		parts := strings.Split(fullpkg, string(filepath.Separator))
		if len(parts) == 0 {
			log.Fatalf("no package to retrieve: %v", fullpkg)
		}
		domain := parts[0]
		if !strings.Contains(domain, ".") {
			log.Fatalf("first part of package path should be a domain name", fullpkg)
		}
		fmt.Println("domain", domain)
		sshURL := strings.Join([]string{domain, filepath.Join(parts[1:]...)}, ":")
		sshURL = "git@" + sshURL + ".git"
		args := []string{"clone", sshURL, arg}
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			log.Fatal("error running git %v: %v", strings.Join(args, " "), err)
		}
	} else {
	}
}
