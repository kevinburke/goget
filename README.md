# goget

Clone Go packages into `$GOPATH/src` using the old GOPATH directory layout. Go
has moved away from this file layout, but it's still a nice way to organize
source code on disk.

## Installation

```
go install github.com/kevinburke/goget@latest
```

## Usage

Ensure you have a `GOPATH` set. Then use `goget` the same way you used `go get`:

```bash
goget github.com/kevinburke/rest
# clones into $GOPATH/src/github.com/kevinburke/rest
```

Subpackage paths are handled correctly -- only the repository root is cloned:

```bash
goget github.com/kevinburke/rest/restclient
# clones github.com/kevinburke/rest into $GOPATH/src/github.com/kevinburke/rest
```

The `/...` suffix is accepted and stripped (for compatibility with old `go get`
invocations):

```bash
goget github.com/kevinburke/rest/...
# same as: goget github.com/kevinburke/rest
```

If the target directory already exists (detected via `go.mod` or `.git`), the
clone is skipped.

### Cloning from a go.mod file

Use `--mod` to fetch all dependencies listed in a `go.mod` file. Dependencies
are cloned in parallel (up to 10 at a time):

```bash
goget --mod go.mod
```

### Flags

```
--https             Use HTTPS for git clones instead of SSH
--mod <path>        Path to a go.mod file; fetch all dependencies
--accept-ssh-host   Automatically accept new SSH host keys
--skip-fsck         Skip fsck checks during clone
```

## Clone behavior

By default, `goget` clones over SSH (`git@host:user/repo.git`). If the SSH
clone fails (e.g. no SSH key configured for that host), it automatically falls
back to HTTPS. Use `--https` to skip SSH and go straight to HTTPS.

### Supported hosts and import paths

| Import path                          | Cloned from                                    |
|--------------------------------------|------------------------------------------------|
| `github.com/user/repo`              | `git@github.com:user/repo.git`                 |
| `gitlab.com/user/repo`              | `git@gitlab.com:user/repo.git`                 |
| `bitbucket.org/user/repo`           | `git@bitbucket.org:user/repo.git`              |
| `golang.org/x/sync`                 | `https://go.googlesource.com/sync`             |
| `google.golang.org/protobuf`        | `https://github.com/googleapis/protobuf`       |
| `go.opentelemetry.io/otel`          | `https://github.com/open-telemetry/otel`       |
| Custom domains (e.g. `example.com`) | Discovered via `<meta name="go-import">` tags  |

For custom domains, `goget` fetches `https://<import-path>?go-get=1` and parses
the `go-import` meta tag to find the repository URL, the same mechanism that `go
get` uses.
