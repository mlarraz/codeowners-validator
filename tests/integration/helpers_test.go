//go:build integration

package integration

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func normalizeTimeDurations(in string) string {
	duration := regexp.MustCompile(`\(\d+(\.\d+)?(ns|us|µs|ms|s|m|h)\)`)
	return duration.ReplaceAllString(in, "(<duration>)")
}

func normalizeLogTime(in string) string {
	duration := regexp.MustCompile(`time="[^"]*"`)
	return duration.ReplaceAllString(in, `time="<time>"`)
}

// getEnvOrSkip returns the value of the given environment variable and skips the
// test when it's empty. These variables hold credentials for the test organizations,
// which aren't available everywhere the tests run, for example in forks.
func getEnvOrSkip(t *testing.T, name string) string {
	t.Helper()

	value := os.Getenv(name)
	if value == "" {
		t.Skipf("%s is not set", name)
	}
	return value
}

// SampleRepo copies the sample repository testdata/repos/<org>/<branch> into a
// temporary directory, commits all of its files to a new Git repository, and
// returns the directory.
//
// The samples store their .gitignore files as "gitignore" so that they don't hide
// sample files from this repository's own Git index; they're renamed back here.
// Files are force-added because the samples contain files that their own
// .gitignore excludes.
func SampleRepo(t *testing.T, org string, branch string) string {
	t.Helper()

	repoDir := t.TempDir()
	require.NoError(t, os.CopyFS(repoDir, os.DirFS(filepath.Join("testdata", "repos", org, branch))))
	require.NoError(t, filepath.WalkDir(repoDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() != "gitignore" {
			return err
		}
		return os.Rename(path, filepath.Join(filepath.Dir(path), ".gitignore"))
	}))

	for _, args := range [][]string{
		{"init", "--quiet"},
		{"add", "--all", "--force"},
		{"-c", "user.name=test", "-c", "user.email=test@example.com", "-c", "commit.gpgsign=false", "commit", "--quiet", "--message", "sample"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = repoDir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
	}

	return repoDir
}

// BloblessSparseClone creates a clone of the Git repository in origin that fetches no file
// contents and checks out only the given paths, and returns the directory of the clone.
func BloblessSparseClone(t *testing.T, origin string, paths ...string) string {
	t.Helper()

	git(t, origin, "config", "uploadpack.allowFilter", "true")

	cloneDir := filepath.Join(t.TempDir(), "clone")
	git(t, "", "clone", "--quiet", "--no-local", "--filter=blob:none", "--no-checkout", "file://"+filepath.ToSlash(origin), cloneDir)
	git(t, cloneDir, append([]string{"sparse-checkout", "set", "--no-cone"}, paths...)...)
	git(t, cloneDir, "checkout", "--quiet")

	return cloneDir
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
}

type Executor struct {
	envs       map[string]string
	timeout    time.Duration
	binaryPath string
	args       []string
}

func Exec() *Executor {
	return &Executor{
		envs: map[string]string{},
	}
}

// WithEnv adds given env. Overrides if previously existed
func (s *Executor) WithEnv(key string, value string) *Executor {
	s.envs[key] = value
	return s
}

type ExecuteOutput struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Err      error
}

func (s *Executor) AwaitResultAtMost(timeout time.Duration) *ExecuteOutput {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var stdout, stderr bytes.Buffer

	cmd := exec.CommandContext(ctx, s.binaryPath, s.args...)
	cmd.Stderr = &stderr
	cmd.Stdout = &stdout

	for k, v := range s.envs {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	err := cmd.Run()
	return &ExecuteOutput{
		ExitCode: cmd.ProcessState.ExitCode(),
		Err:      err,
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
	}
}

func (s *Executor) WithTimeout(timeout time.Duration) *Executor {
	s.timeout = timeout
	return s
}

func (s *Executor) Binary(binaryPath string) *Executor {
	s.binaryPath = binaryPath
	return s
}

// WithArgs sets the command-line arguments.
func (s *Executor) WithArgs(args ...string) *Executor {
	s.args = args
	return s
}
