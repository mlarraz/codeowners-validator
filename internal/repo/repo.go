// Package repo reads the tracked files and the checked-out commit of the repository being validated.
//
// It reads Git's index and object database instead of the working tree, so it works on sparse and
// blobless (`--filter=blob:none`) checkouts that only contain the CODEOWNERS file.
package repo

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// ErrNotRepository is returned by Head when the directory isn't in a Git repository.
var ErrNotRepository = errors.New("not a Git repository")

// Repo is the repository being validated. It's safe for concurrent use.
type Repo struct {
	dir   string
	trust bool

	filesOnce sync.Once
	files     []string
	filesErr  error
}

// New returns the repository in dir. If trust is set, Git commands mark dir as a safe directory,
// which is needed when dir is owned by another user, for example a workspace mounted into a
// container. See https://github.com/actions/checkout/issues/766.
func New(dir string, trust bool) *Repo {
	return &Repo{dir: dir, trust: trust}
}

// Dir returns the repository directory.
func (r *Repo) Dir() string {
	return r.dir
}

// Files returns the slash-separated paths of the files tracked in the repository's index, relative
// to the repository directory and sorted bytewise. If the directory isn't in a Git repository, it
// returns the files in the directory instead, skipping .git directories.
//
// The result is computed once and shared; callers must not modify it.
func (r *Repo) Files(ctx context.Context) ([]string, error) {
	r.filesOnce.Do(func() {
		r.files, r.filesErr = r.listFiles(ctx)
	})
	return r.files, r.filesErr
}

func (r *Repo) listFiles(ctx context.Context) ([]string, error) {
	out, err := r.git(ctx, "ls-files", "-z")
	switch {
	case errors.Is(err, ErrNotRepository), errors.Is(err, exec.ErrNotFound):
		return r.walkFiles()
	case err != nil:
		return nil, err
	}

	files := strings.Split(strings.TrimSuffix(string(out), "\x00"), "\x00")
	if len(files) == 1 && files[0] == "" {
		return nil, nil
	}
	// Git sorts the index bytewise already; sort anyway so callers can rely on it.
	sort.Strings(files)
	return files, nil
}

func (r *Repo) walkFiles() ([]string, error) {
	var files []string
	err := filepath.WalkDir(r.dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(r.dir, path)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("while listing files in %s: %w", r.dir, err)
	}
	sort.Strings(files)
	return files, nil
}

// Head returns the SHA of the commit checked out in the repository, or ErrNotRepository if the
// directory isn't in a Git repository.
func (r *Repo) Head(ctx context.Context) (string, error) {
	out, err := r.git(ctx, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func (r *Repo) git(ctx context.Context, args ...string) ([]byte, error) {
	gitArgs := []string{"-C", r.dir}
	if r.trust {
		gitArgs = append(gitArgs, "-c", "safe.directory="+r.dir)
	}
	cmd := exec.CommandContext(ctx, "git", append(gitArgs, args...)...) //nolint:gosec // G204: fixed git binary; the arguments are built by this package, not a shell.

	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err == nil {
		return out, nil
	}

	msg := strings.TrimSpace(stderr.String())
	switch {
	case errors.Is(err, exec.ErrNotFound):
		return nil, err
	case strings.Contains(msg, "not a git repository"):
		return nil, ErrNotRepository
	case strings.Contains(msg, "dubious ownership"):
		return nil, fmt.Errorf("git %s: %s\nSet TRUST_WORKSPACE=true to trust %s", strings.Join(args, " "), msg, r.dir)
	default:
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, msg)
	}
}
