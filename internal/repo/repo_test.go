package repo_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mlarraz/codeowners-validator/internal/repo"
)

func TestFilesListsTrackedFilesOfSparseBloblessClone(t *testing.T) {
	origin := t.TempDir()
	writeFiles(t, origin, ".github/CODEOWNERS", "docs/a b.md", "src/main.go", "src/pkg/util.go")
	git(t, origin, "init", "--quiet")
	git(t, origin, "add", "--all")
	git(t, origin, "-c", "user.name=test", "-c", "user.email=test@example.com", "-c", "commit.gpgsign=false", "commit", "--quiet", "--message", "init")
	git(t, origin, "config", "uploadpack.allowFilter", "true")

	clone := filepath.Join(t.TempDir(), "clone")
	git(t, "", "clone", "--quiet", "--no-local", "--filter=blob:none", "--no-checkout", "file://"+filepath.ToSlash(origin), clone)
	git(t, clone, "sparse-checkout", "set", "--no-cone", ".github/CODEOWNERS")
	git(t, clone, "checkout", "--quiet")
	require.NoFileExists(t, filepath.Join(clone, "src", "main.go"))

	files, err := repo.New(clone, false).Files(context.Background())

	require.NoError(t, err)
	assert.Equal(t, []string{".github/CODEOWNERS", "docs/a b.md", "src/main.go", "src/pkg/util.go"}, files)
}

func TestFilesListsDirectoryOutsideGitRepository(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, "CODEOWNERS", "src/main.go", ".git/config")

	files, err := repo.New(dir, false).Files(context.Background())

	require.NoError(t, err)
	assert.Equal(t, []string{"CODEOWNERS", "src/main.go"}, files)
}

func TestHeadOutsideGitRepository(t *testing.T) {
	_, err := repo.New(t.TempDir(), false).Head(context.Background())

	assert.ErrorIs(t, err, repo.ErrNotRepository)
}

func writeFiles(t *testing.T, dir string, names ...string) {
	t.Helper()
	for _, name := range names {
		path := filepath.Join(dir, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(name+"\n"), 0o600))
	}
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
}
