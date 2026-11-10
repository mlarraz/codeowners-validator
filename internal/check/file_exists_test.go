package check_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/mlarraz/codeowners-validator/internal/check"
	"github.com/mlarraz/codeowners-validator/internal/ptr"
	"github.com/mlarraz/codeowners-validator/internal/repo"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// docSampleFiles are the files used by the examples of CODEOWNERS patterns in GitHub's
// documentation.
// https://docs.github.com/en/repositories/managing-your-repositorys-settings-and-features/customizing-your-repository/about-code-owners#example-of-a-codeowners-file
var docSampleFiles = []string{
	"app.js",
	"src/app.js",
	"build/logs/console.log",
	"build/other/cache.log",
	"docs/a.md",
	"docs/nested/b.md",
	"apps/web/index.html",
	"apps/github/main.go",
	"scripts/release.sh",
	"logs/boot.log",
	"services/api/logs/debug.log",
	"a/b/file.txt",
	"a/bc/file.txt",
}

func TestFileExistMatchesFilesWithGitHubSemantics(t *testing.T) {
	repository := newTestRepo(t, docSampleFiles...)

	tests := []struct {
		name    string
		pattern string
		matched bool
	}{
		{name: "wildcard matches files at any depth", pattern: "*.js", matched: true},
		{name: "wildcard for suffix", pattern: "*.rs", matched: false},

		{name: "anchored directory with trailing slash", pattern: "/build/logs/", matched: true},
		{name: "anchored directory without trailing slash", pattern: "/build/logs", matched: true},
		{name: "anchored directory glob", pattern: "/build/*/cache.log", matched: true},
		{name: "directory glob doesn't match at depth", pattern: "docs/*/b.md", matched: true},
		{name: "directory glob doesn't match nested file", pattern: "docs/*", matched: true},

		{name: "single segment pattern matches at any depth", pattern: "apps/", matched: true},
		{name: "single segment pattern with wildcard matches at any depth", pattern: "logs/", matched: true},
		{name: "double star matches directory at any depth", pattern: "**/logs", matched: true},

		{name: "anchored directory", pattern: "/apps/", matched: true},
		{name: "anchored file", pattern: "/apps/github/main.go", matched: true},
		{name: "anchored directory without trailing slash", pattern: "/apps/github", matched: true},
		{name: "anchored directory without owners", pattern: "/scripts/", matched: true},

		{name: "literal file", pattern: "/app.js", matched: true},
		{name: "literal path doesn't match a path that it prefixes", pattern: "/app", matched: false},
		{name: "literal directory", pattern: "/a/b", matched: true},
		{name: "literal directory doesn't match a sibling that shares the prefix", pattern: "/a/b/", matched: true},
		{name: "literal prefix isn't a path prefix", pattern: "/a/bc", matched: true},
		{name: "literal file doesn't match a file that it prefixes", pattern: "/a/b/file", matched: false},
		{name: "literal file", pattern: "/a/b/file.txt", matched: true},
		{name: "unanchored multi-segment pattern is anchored at the root", pattern: "a/bc", matched: true},
		{name: "unanchored multi-segment pattern doesn't match a nested path", pattern: "nested/b.md", matched: false},
		{name: "unanchored multi-segment file pattern", pattern: "docs/nested/b.md", matched: true},

		{name: "patterns are case-sensitive", pattern: "/Build/", matched: false},
		{name: "root pattern matches nothing", pattern: "/", matched: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// given
			in := LoadInput(tc.pattern + " @sample-owner")
			in.Repo = repository

			// when
			out, err := check.NewFileExist().Check(context.Background(), in)

			// then
			require.NoError(t, err)
			if tc.matched {
				assert.Empty(t, out.Issues)
				return
			}
			require.Len(t, out.Issues, 1)
			assert.Equal(t, issueAt(1, fmt.Sprintf("%q does not match any files in repository", tc.pattern), tc.pattern), out.Issues[0])
		})
	}
}

func TestFileExistLiteralDirectoryBoundary(t *testing.T) {
	tests := []struct {
		name    string
		files   []string
		pattern string
		matched bool
	}{
		{
			name:    "literal directory doesn't match a sibling that shares its prefix",
			files:   []string{"a/bc/file.txt"},
			pattern: "/a/b",
			matched: false,
		},
		{
			name:    "literal directory with trailing slash doesn't match a sibling that shares its prefix",
			files:   []string{"a/bc/file.txt"},
			pattern: "/a/b/",
			matched: false,
		},
		{
			name:    "literal directory matches its files",
			files:   []string{"a/b/file.txt"},
			pattern: "/a/b",
			matched: true,
		},
		{
			name:    "literal directory doesn't match a path that it's a prefix of",
			files:   []string{"a/b/file.txt"},
			pattern: "/a/bc",
			matched: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// given
			repository := newTestRepo(t, tc.files...)
			in := LoadInput(tc.pattern + " @sample-owner")
			in.Repo = repository

			// when
			out, err := check.NewFileExist().Check(context.Background(), in)

			// then
			require.NoError(t, err)
			if tc.matched {
				assert.Empty(t, out.Issues)
				return
			}
			assert.Equal(t, []check.Issue{issueAt(1, fmt.Sprintf("%q does not match any files in repository", tc.pattern), tc.pattern)}, out.Issues)
		})
	}
}

func TestFileExistReportsEveryUnmatchedPattern(t *testing.T) {
	// given
	repository := newTestRepo(t, "app.js", "src/app.js")
	codeowners := "*.js @js-owner\n/missing/ @missing-owner\n*.rs @rust-owner\n"

	// when
	out, err := check.NewFileExist().Check(context.Background(), newInput(repository, codeowners))

	// then
	require.NoError(t, err)
	assert.Equal(t, []check.Issue{
		issueAt(2, `"/missing/" does not match any files in repository`, "/missing/"),
		issueAt(3, `"*.rs" does not match any files in repository`, "*.rs"),
	}, out.Issues)
}

func TestFileExistOnlyMatchesTrackedFiles(t *testing.T) {
	// given
	repository := newTestRepo(t, "app.js")
	addUntrackedFile(t, repository, "notes.txt")
	in := LoadInput("/notes.txt @sample-owner")
	in.Repo = repository

	// when
	out, err := check.NewFileExist().Check(context.Background(), in)

	// then
	require.NoError(t, err)
	assert.Equal(t, []check.Issue{issueAt(1, `"/notes.txt" does not match any files in repository`, "/notes.txt")}, out.Issues)
}

func TestFileExistReportsInvalidPatterns(t *testing.T) {
	tests := []struct {
		name        string
		pattern     string
		wantMessage string
	}{
		{
			name:        "negation isn't supported",
			pattern:     "!/docs/",
			wantMessage: `"!/docs/" is not a valid CODEOWNERS pattern: unexpected character '!' at position 1`,
		},
		{
			name:        "too many asterisks",
			pattern:     "docs***",
			wantMessage: `"docs***" is not a valid CODEOWNERS pattern: pattern cannot contain three consecutive asterisks`,
		},
	}

	repository := newTestRepo(t, "docs/a.md")

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// given
			in := LoadInput(tc.pattern + " @sample-owner")
			in.Repo = repository

			// when
			out, err := check.NewFileExist().Check(context.Background(), in)

			// then
			require.NoError(t, err)
			assert.Equal(t, []check.Issue{issueAt(1, tc.wantMessage, tc.pattern)}, out.Issues)
		})
	}
}

func TestFileExistWorksOutsideGitRepository(t *testing.T) {
	// given
	dir := t.TempDir()
	writeFile(t, dir, "docs/a.md")
	repository := repo.New(dir, false)

	// when
	out, err := check.NewFileExist().Check(context.Background(), newInput(repository, "/docs/ @sample-owner\n/missing/ @missing-owner"))

	// then
	require.NoError(t, err)
	assert.Equal(t, []check.Issue{issueAt(2, `"/missing/" does not match any files in repository`, "/missing/")}, out.Issues)
}

func TestFileExistReturnsErrorWhenFilesCannotBeListed(t *testing.T) {
	// given: the repository path is a file, so it's not a Git repository and can't be walked
	dir := t.TempDir()
	writeFile(t, dir, "codeowners")
	repository := repo.New(filepath.Join(dir, "codeowners"), false)

	// when
	out, err := check.NewFileExist().Check(context.Background(), newInput(repository, "* @sample-owner"))

	// then
	require.Error(t, err)
	assert.Empty(t, out)
}

func newInput(repository *repo.Repo, codeowners string) check.Input {
	in := LoadInput(codeowners)
	in.Repo = repository
	return in
}

func issueAt(line uint64, msg, key string) check.Issue {
	return check.Issue{
		Severity: check.Error,
		LineNo:   ptr.Uint64Ptr(line),
		Message:  msg,
		Key:      key,
	}
}
