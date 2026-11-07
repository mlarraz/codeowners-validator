package check_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mlarraz/codeowners-validator/internal/check"
	"github.com/mlarraz/codeowners-validator/internal/repo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mlarraz/codeowners-validator/pkg/codeowners"
)

var FixtureValidCODEOWNERS = `
		# These owners will be the default owners for everything
		*       @global-owner1 @global-owner2

		# js owner
		*.js    @js-owner

		*.go docs@example.com

		/build/logs/ @doctocat

		/script m.t@g.com
`

func LoadInput(in string) check.Input {
	r := strings.NewReader(in)

	return check.Input{
		CodeownersEntries: codeowners.ParseCodeowners(r),
	}
}

// newTestRepo creates a Git repository that tracks the given files and returns it. Paths are
// slash-separated.
func newTestRepo(t *testing.T, files ...string) *repo.Repo {
	t.Helper()

	dir := t.TempDir()
	for _, file := range files {
		path := filepath.Join(dir, filepath.FromSlash(file))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(file+"\n"), 0o600))
	}

	runGit(t, dir, "init", "--quiet")
	runGit(t, dir, "add", "--all", "--force")
	runGit(t, dir, "-c", "user.name=test", "-c", "user.email=test@example.com", "-c", "commit.gpgsign=false", "commit", "--quiet", "--allow-empty", "--message", "sample")

	return repo.New(dir, false)
}

// runGit runs git with the given arguments in dir and fails the test if it fails.
func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
}

// addUntrackedFile creates a file in the working tree of a repository created with newTestRepo
// without adding it to Git's index.
func addUntrackedFile(t *testing.T, r *repo.Repo, file string) {
	t.Helper()

	writeFile(t, r.Dir(), file)
}

// writeFile creates the slash-separated file under dir.
func writeFile(t *testing.T, dir, file string) {
	t.Helper()

	path := filepath.Join(dir, filepath.FromSlash(file))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(file+"\n"), 0o600))
}

func assertIssue(t *testing.T, expIssue *check.Issue, gotIssues []check.Issue) {
	t.Helper()

	if expIssue != nil {
		require.Len(t, gotIssues, 1)
		assert.EqualValues(t, *expIssue, gotIssues[0])
	} else {
		assert.Empty(t, gotIssues)
	}
}
