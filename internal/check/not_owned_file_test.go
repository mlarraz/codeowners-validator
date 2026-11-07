package check_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/mlarraz/codeowners-validator/internal/check"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNotOwnedFileMatchesFilesWithGitHubSemantics(t *testing.T) {
	// given
	repository := newTestRepo(t,
		"CODEOWNERS",
		"app.js",
		"src/app.js",
		"build/logs/console.log",
		"docs/a.md",
		"docs/nested/b.md",
		"apps/github/main.go",
		"apps/web/index.html",
		"scripts/release.sh",
		"logs/boot.log",
		"services/api/logs/debug.log",
	)
	codeowners := strings.Join([]string{
		"*.js @js-owner",
		"/build/logs/ @ci-owner",
		"docs/* @docs-owner",
		"apps/ @apps-owner",
		"/scripts/ @release-owner",
		"**/logs @logs-owner",
		"/apps/github",
	}, "\n")

	// when
	out, err := check.NewNotOwnedFile(check.NotOwnedFileConfig{}).Check(context.Background(), newInput(repository, codeowners))

	// then
	require.NoError(t, err)
	assert.Equal(t, []check.Issue{
		notOwnedIssue("", "CODEOWNERS", "docs/nested/b.md"),
	}, out.Issues)
}

func TestNotOwnedFileSkipsPatterns(t *testing.T) {
	repository := newTestRepo(t, "docs/a.md", "src/main.go")
	codeowners := "* @global-owner\n/docs/ @docs-owner\n"

	tests := []struct {
		name           string
		skipPatterns   []string
		wantUnowned    []string
		wantSkippedFmt string
	}{
		{
			name:           "skipping an entry makes its files not owned",
			skipPatterns:   []string{"*"},
			wantUnowned:    []string{"src/main.go"},
			wantSkippedFmt: "*",
		},
		{
			name:           "skipped patterns are listed in sorted order",
			skipPatterns:   []string{"/docs/", "*"},
			wantUnowned:    []string{"docs/a.md", "src/main.go"},
			wantSkippedFmt: "*,/docs/",
		},
		{
			name:           "skipping a pattern that isn't an entry changes nothing",
			skipPatterns:   []string{"/unused/"},
			wantUnowned:    nil,
			wantSkippedFmt: "/unused/",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// given
			checker := check.NewNotOwnedFile(check.NotOwnedFileConfig{SkipPatterns: tc.skipPatterns})

			// when
			out, err := checker.Check(context.Background(), newInput(repository, codeowners))

			// then
			require.NoError(t, err)
			if len(tc.wantUnowned) == 0 {
				assert.Empty(t, out.Issues)
				return
			}
			assert.Equal(t, []check.Issue{notOwnedIssue(tc.wantSkippedFmt, tc.wantUnowned...)}, out.Issues)
		})
	}
}

func TestNotOwnedFileSubdirectories(t *testing.T) {
	repository := newTestRepo(t, "README.md", "tools/a.sh", "tools/sub/b.sh", "tools2/c.sh")
	codeowners := "/tools/a.sh @tools-owner\n"

	tests := []struct {
		name           string
		subdirectories []string
		wantUnowned    []string
	}{
		{
			name:        "no subdirectories considers every file",
			wantUnowned: []string{"README.md", "tools/sub/b.sh", "tools2/c.sh"},
		},
		{
			name:           "a subdirectory covers only the files under it",
			subdirectories: []string{"tools"},
			wantUnowned:    []string{"tools/sub/b.sh"},
		},
		{
			name:           "a subdirectory doesn't cover a sibling that shares its prefix",
			subdirectories: []string{"tools2"},
			wantUnowned:    []string{"tools2/c.sh"},
		},
		{
			name:           "a trailing slash is ignored",
			subdirectories: []string{"tools/"},
			wantUnowned:    []string{"tools/sub/b.sh"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// given
			checker := check.NewNotOwnedFile(check.NotOwnedFileConfig{Subdirectories: tc.subdirectories})

			// when
			out, err := checker.Check(context.Background(), newInput(repository, codeowners))

			// then
			require.NoError(t, err)
			assert.Equal(t, []check.Issue{notOwnedIssue("", tc.wantUnowned...)}, out.Issues)
		})
	}
}

func TestNotOwnedFileLiteralDirectoryBoundary(t *testing.T) {
	// given: /a/b must not own the files of a/bc
	repository := newTestRepo(t, "a/b/file.txt", "a/bc/file.txt")
	codeowners := "/a/b @sample-owner\n"

	// when
	out, err := check.NewNotOwnedFile(check.NotOwnedFileConfig{}).Check(context.Background(), newInput(repository, codeowners))

	// then
	require.NoError(t, err)
	assert.Equal(t, []check.Issue{notOwnedIssue("", "a/bc/file.txt")}, out.Issues)
}

func TestNotOwnedFileConsidersAllEntries(t *testing.T) {
	// given: the last matching entry has no owners, which doesn't matter for the check
	repository := newTestRepo(t, "src/ignored/x.go")
	codeowners := "src/ @src-owner\nsrc/ignored\n"

	// when
	out, err := check.NewNotOwnedFile(check.NotOwnedFileConfig{}).Check(context.Background(), newInput(repository, codeowners))

	// then
	require.NoError(t, err)
	assert.Empty(t, out.Issues)
}

func TestNotOwnedFileMarksLiteralPatternsBeforeWildcardPatterns(t *testing.T) {
	// given
	repository := newTestRepo(t, "apps/x/secret", "apps/y/secret", "apps/z/public")
	codeowners := "/apps/x/ @x-owner\n/apps/*/secret @secret-owner\n"

	// when
	out, err := check.NewNotOwnedFile(check.NotOwnedFileConfig{}).Check(context.Background(), newInput(repository, codeowners))

	// then
	require.NoError(t, err)
	assert.Equal(t, []check.Issue{notOwnedIssue("", "apps/z/public")}, out.Issues)
}

func TestNotOwnedFileIgnoresInvalidPatterns(t *testing.T) {
	// given
	repository := newTestRepo(t, "docs/a.md", "src/main.go")
	codeowners := "!/docs/ @sample-owner\n/docs/ @docs-owner\n"

	// when
	out, err := check.NewNotOwnedFile(check.NotOwnedFileConfig{}).Check(context.Background(), newInput(repository, codeowners))

	// then
	require.NoError(t, err)
	assert.Equal(t, []check.Issue{notOwnedIssue("", "src/main.go")}, out.Issues)
}

func TestNotOwnedFileReportsEmptyCODEOWNERS(t *testing.T) {
	// given
	repository := newTestRepo(t, "docs/a.md")

	// when
	out, err := check.NewNotOwnedFile(check.NotOwnedFileConfig{}).Check(context.Background(), newInput(repository, "# only a comment\n"))

	// then
	require.NoError(t, err)
	assert.Equal(t, []check.Issue{{
		Severity: check.Error,
		Message:  "The CODEOWNERS file is empty. The files in the repository don't have any owner.",
	}}, out.Issues)
}

func TestNotOwnedFileOnlyConsidersTrackedFiles(t *testing.T) {
	// given
	repository := newTestRepo(t, "docs/a.md")
	addUntrackedFile(t, repository, "docs/untracked.md")

	// when
	out, err := check.NewNotOwnedFile(check.NotOwnedFileConfig{}).Check(context.Background(), newInput(repository, "/docs/ @docs-owner\n"))

	// then
	require.NoError(t, err)
	assert.Empty(t, out.Issues)
}

// notOwnedIssue returns the issue that the not owned files check reports for the given files.
func notOwnedIssue(skippedPatterns string, files ...string) check.Issue {
	points := make([]string, len(files))
	for i, file := range files {
		points[i] = "            * " + file
	}

	return check.Issue{
		Severity: check.Error,
		Message:  fmt.Sprintf("Found %d not owned files (skipped patterns: %q):\n%s", len(files), skippedPatterns, strings.Join(points, "\n")),
	}
}
