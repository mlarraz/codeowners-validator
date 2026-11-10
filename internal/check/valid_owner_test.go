package check_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/go-github/v93/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mlarraz/codeowners-validator/internal/check"
	"github.com/mlarraz/codeowners-validator/internal/ptr"
	"github.com/mlarraz/codeowners-validator/internal/repo"
	"github.com/mlarraz/codeowners-validator/pkg/codeowners"
)

// codeownersErrorsAPI is a fake GitHub API that serves the CODEOWNERS errors endpoint.
type codeownersErrorsAPI struct {
	status   int
	body     string
	requests []*http.Request
}

func (a *codeownersErrorsAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.requests = append(a.requests, r)

	if got, want := r.URL.Path, "/repos/org/repo/codeowners/errors"; got != want {
		a.requestError(w, "unexpected path: "+got)
		return
	}

	if a.status == http.StatusForbidden {
		w.Header().Set("X-RateLimit-Limit", "60")
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", "4102444800")
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(a.status)
	_, _ = w.Write([]byte(a.body))
}

func (a *codeownersErrorsAPI) requestError(w http.ResponseWriter, msg string) {
	http.Error(w, msg, http.StatusInternalServerError)
}

func (a *codeownersErrorsAPI) refs() []string {
	refs := make([]string, 0, len(a.requests))
	for _, r := range a.requests {
		refs = append(refs, r.URL.Query().Get("ref"))
	}
	return refs
}

func newOwnerCheck(t *testing.T, cfg check.ValidOwnerConfig, api http.Handler) *check.ValidOwner {
	t.Helper()

	srv := httptest.NewServer(api)
	t.Cleanup(srv.Close)

	baseURL := srv.URL + "/"
	ghClient, err := github.NewClient(
		github.WithHTTPClient(srv.Client()),
		github.WithURLs(&baseURL, &baseURL),
	)
	require.NoError(t, err)

	if cfg.Repository == "" {
		cfg.Repository = "org/repo"
	}

	ownerCheck, err := check.NewValidOwner(cfg, ghClient)
	require.NoError(t, err)
	return ownerCheck
}

func parseEntries(codeownersFile string) []codeowners.Entry {
	return codeowners.ParseCodeowners(strings.NewReader(codeownersFile))
}

func TestValidOwnerCheckerRepositoryName(t *testing.T) {
	tests := map[string]struct {
		repository string
		wantErr    bool
	}{
		"Empty":      {repository: "", wantErr: true},
		"Owner only": {repository: "org", wantErr: true},
		"Empty repo": {repository: "org/", wantErr: true},
		"Too many segments": {
			repository: "org/repo/extra",
			wantErr:    true,
		},
		"Owner and repo": {repository: "org/repo"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := check.NewValidOwner(check.ValidOwnerConfig{Repository: tc.repository}, nil)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestValidOwnerCheckerReportsGitHubErrors(t *testing.T) {
	api := &codeownersErrorsAPI{
		status: http.StatusOK,
		body: `{
			"errors": [
				{
					"line": 10,
					"column": 29,
					"source": "/some/awesome/dir @owner-ok @owner-a\n",
					"kind": "Unknown owner",
					"suggestion": "make sure @owner-a exists and has write access to the repository",
					"message": "Unknown owner on line 10: make sure @owner-a exists and has write access to the repository",
					"path": "CODEOWNERS"
				},
				{
					"line": 15,
					"column": 15,
					"source": "/some/other/file @org/team\n",
					"kind": "Unknown owner",
					"suggestion": "make sure the team @org/team exists, is publicly visible, and has write access to the repository",
					"message": "Unknown owner on line 15",
					"path": "CODEOWNERS"
				},
				{
					"line": 16,
					"column": 1,
					"source": "docs@ @owner-a\n",
					"kind": "Invalid owner",
					"message": "Invalid owner on line 16",
					"path": "CODEOWNERS"
				},
				{
					"line": 17,
					"column": 6,
					"source": "*.rb @owner-a\n",
					"kind": "Invalid pattern",
					"suggestion": "did you mean ` + "`**/*.rb`" + ` instead?",
					"message": "Invalid pattern on line 17",
					"path": "CODEOWNERS"
				},
				{
					"line": 18,
					"column": 99,
					"source": "@owner-a\n",
					"kind": "Invalid owner",
					"message": "Invalid owner on line 18",
					"path": "CODEOWNERS"
				}
			]
		}`,
	}

	// given
	ownerCheck := newOwnerCheck(t, check.ValidOwnerConfig{IgnoredOwners: []string{"@ghost"}}, api)

	// when
	out, err := ownerCheck.Check(context.Background(), check.Input{
		CodeownersEntries: parseEntries("* @owner-a\n"),
	})

	// then
	require.NoError(t, err)
	assert.Equal(t, []check.Issue{
		{
			Severity: check.Error,
			LineNo:   ptr.Uint64Ptr(10),
			Message:  "Unknown owner: make sure @owner-a exists and has write access to the repository",
			Key:      "Unknown owner: make sure @owner-a exists and has write access to the repository: /some/awesome/dir @owner-ok @owner-a",
		},
		{
			Severity: check.Error,
			LineNo:   ptr.Uint64Ptr(15),
			Message:  "Unknown owner: make sure the team @org/team exists, is publicly visible, and has write access to the repository",
			Key:      "Unknown owner: make sure the team @org/team exists, is publicly visible, and has write access to the repository: /some/other/file @org/team",
		},
		{
			Severity: check.Error,
			LineNo:   ptr.Uint64Ptr(16),
			Message:  "Invalid owner: docs@",
			Key:      "Invalid owner: docs@: docs@ @owner-a",
		},
		{
			Severity: check.Error,
			LineNo:   ptr.Uint64Ptr(17),
			Message:  "Invalid pattern: did you mean `**/*.rb` instead?",
			Key:      "Invalid pattern: did you mean `**/*.rb` instead?: *.rb @owner-a",
		},
		{
			Severity: check.Error,
			LineNo:   ptr.Uint64Ptr(18),
			Message:  "Invalid owner",
			Key:      "Invalid owner: @owner-a",
		},
	}, out.Issues)

	require.Len(t, api.requests, 1, "the CODEOWNERS errors endpoint must be called once")
}

func TestValidOwnerCheckerIgnoredOwners(t *testing.T) {
	api := &codeownersErrorsAPI{
		status: http.StatusOK,
		body: `{"errors": [
			{"line": 1, "column": 6, "source": "*.md @ghost\n", "kind": "Unknown owner", "suggestion": "make sure @ghost exists and has write access to the repository"},
			{"line": 2, "column": 6, "source": "*.go @GHOST\n", "kind": "Unknown owner", "suggestion": "make sure @GHOST exists and has write access to the repository"},
			{"line": 3, "column": 6, "source": "*.rb @owner-a\n", "kind": "Unknown owner", "suggestion": "make sure @owner-a exists and has write access to the repository"}
		]}`,
	}

	tests := map[string]struct {
		ignoredOwners []string
		wantIssues    []check.Issue
	}{
		"Case-insensitive match against the configured owners": {
			ignoredOwners: []string{"@Ghost"},
			wantIssues: []check.Issue{
				{
					Severity: check.Error,
					LineNo:   ptr.Uint64Ptr(3),
					Message:  "Unknown owner: make sure @owner-a exists and has write access to the repository",
					Key:      "Unknown owner: make sure @owner-a exists and has write access to the repository: *.rb @owner-a",
				},
			},
		},
		"No ignored owners": {
			ignoredOwners: []string{},
			wantIssues: []check.Issue{
				{
					Severity: check.Error,
					LineNo:   ptr.Uint64Ptr(1),
					Message:  "Unknown owner: make sure @ghost exists and has write access to the repository",
					Key:      "Unknown owner: make sure @ghost exists and has write access to the repository: *.md @ghost",
				},
				{
					Severity: check.Error,
					LineNo:   ptr.Uint64Ptr(2),
					Message:  "Unknown owner: make sure @GHOST exists and has write access to the repository",
					Key:      "Unknown owner: make sure @GHOST exists and has write access to the repository: *.go @GHOST",
				},
				{
					Severity: check.Error,
					LineNo:   ptr.Uint64Ptr(3),
					Message:  "Unknown owner: make sure @owner-a exists and has write access to the repository",
					Key:      "Unknown owner: make sure @owner-a exists and has write access to the repository: *.rb @owner-a",
				},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// given
			ownerCheck := newOwnerCheck(t, check.ValidOwnerConfig{IgnoredOwners: tc.ignoredOwners}, api)

			// when
			out, err := ownerCheck.Check(context.Background(), check.Input{
				CodeownersEntries: parseEntries("* @ghost\n"),
			})

			// then
			require.NoError(t, err)
			assert.Equal(t, tc.wantIssues, out.Issues)
		})
	}
}

func TestValidOwnerCheckerLocalRules(t *testing.T) {
	api := &codeownersErrorsAPI{status: http.StatusOK, body: `{"errors": []}`}

	tests := map[string]struct {
		cfg        check.ValidOwnerConfig
		codeowners string
		wantIssues []check.Issue
	}{
		"Unowned pattern is reported when not allowed": {
			cfg:        check.ValidOwnerConfig{AllowUnownedPatterns: false},
			codeowners: "*\n/docs/ @org/team\n",
			wantIssues: []check.Issue{
				{
					Severity: check.Warning,
					LineNo:   ptr.Uint64Ptr(1),
					Message:  "Missing owner, at least one owner is required",
					Key:      "Missing owner, at least one owner is required: *",
				},
			},
		},
		"Unowned pattern is allowed": {
			cfg:        check.ValidOwnerConfig{AllowUnownedPatterns: true},
			codeowners: "*\n",
		},
		"Non-team owners are reported when teams are required": {
			cfg:        check.ValidOwnerConfig{AllowUnownedPatterns: true, OwnersMustBeTeams: true},
			codeowners: "*\t@owner1 @org/team user@example.com\n",
			wantIssues: []check.Issue{
				{
					Severity: check.Error,
					LineNo:   ptr.Uint64Ptr(1),
					Message:  `Only team owners allowed and "@owner1" is not a team`,
					Key:      `Only team owners allowed and "@owner1" is not a team: * @owner1 @org/team user@example.com`,
				},
				{
					Severity: check.Error,
					LineNo:   ptr.Uint64Ptr(1),
					Message:  `Only team owners allowed and "user@example.com" is not a team`,
					Key:      `Only team owners allowed and "user@example.com" is not a team: * @owner1 @org/team user@example.com`,
				},
			},
		},
		"Malformed team is reported when teams are required": {
			cfg:        check.ValidOwnerConfig{AllowUnownedPatterns: true, OwnersMustBeTeams: true},
			codeowners: "*\t@org/\n",
			wantIssues: []check.Issue{
				{
					Severity: check.Error,
					LineNo:   ptr.Uint64Ptr(1),
					Message:  `Only team owners allowed and "@org/" is not a team`,
					Key:      `Only team owners allowed and "@org/" is not a team: * @org/`,
				},
			},
		},
		"Ignored owners are exempt from the local rules": {
			cfg: check.ValidOwnerConfig{
				AllowUnownedPatterns: true,
				OwnersMustBeTeams:    true,
				IgnoredOwners:        []string{"@Owner1"},
			},
			codeowners: "*\t@owner1 @org/team\n",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// given
			ownerCheck := newOwnerCheck(t, tc.cfg, api)

			// when
			out, err := ownerCheck.Check(context.Background(), check.Input{
				CodeownersEntries: parseEntries(tc.codeowners),
			})

			// then
			require.NoError(t, err)
			assert.Equal(t, tc.wantIssues, out.Issues)
		})
	}
}

func TestValidOwnerCheckerRef(t *testing.T) {
	t.Run("Defaults to the checked-out commit", func(t *testing.T) {
		// given
		repository := newTestRepo(t, "CODEOWNERS")
		checkedOut, err := repository.Head(context.Background())
		require.NoError(t, err)

		api := &codeownersErrorsAPI{status: http.StatusOK, body: `{"errors": []}`}
		ownerCheck := newOwnerCheck(t, check.ValidOwnerConfig{}, api)

		// when
		out, err := ownerCheck.Check(context.Background(), check.Input{
			Repo:              repository,
			CodeownersEntries: parseEntries("* @owner-a\n"),
		})

		// then
		require.NoError(t, err)
		assert.Empty(t, out.Issues)
		assert.Equal(t, []string{checkedOut}, api.refs())
	})

	t.Run("Explicit ref wins over the checked-out commit", func(t *testing.T) {
		// given
		repository := newTestRepo(t, "CODEOWNERS")

		api := &codeownersErrorsAPI{status: http.StatusOK, body: `{"errors": []}`}
		ownerCheck := newOwnerCheck(t, check.ValidOwnerConfig{Ref: "happy-path"}, api)

		// when
		_, err := ownerCheck.Check(context.Background(), check.Input{
			Repo:              repository,
			CodeownersEntries: parseEntries("* @owner-a\n"),
		})

		// then
		require.NoError(t, err)
		assert.Equal(t, []string{"happy-path"}, api.refs())
	})

	t.Run("No ref is sent outside a Git repository", func(t *testing.T) {
		// given
		api := &codeownersErrorsAPI{status: http.StatusOK, body: `{"errors": []}`}
		ownerCheck := newOwnerCheck(t, check.ValidOwnerConfig{}, api)

		// when
		_, err := ownerCheck.Check(context.Background(), check.Input{
			Repo:              repo.New(t.TempDir(), false),
			CodeownersEntries: parseEntries("* @owner-a\n"),
		})

		// then
		require.NoError(t, err)
		assert.Equal(t, []string{""}, api.refs())

		for _, r := range api.requests {
			assert.NotContains(t, r.URL.RawQuery, "ref=", "GitHub must fall back to the default branch")
		}
	})

	t.Run("Fails when the checked-out commit cannot be read", func(t *testing.T) {
		// given
		dir := t.TempDir()
		runGit(t, dir, "init", "--quiet") // repository without any commit, so HEAD cannot be resolved

		api := &codeownersErrorsAPI{status: http.StatusOK, body: `{"errors": []}`}
		ownerCheck := newOwnerCheck(t, check.ValidOwnerConfig{}, api)

		// when
		_, err := ownerCheck.Check(context.Background(), check.Input{
			Repo:              repo.New(dir, false),
			CodeownersEntries: parseEntries("* @owner-a\n"),
		})

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "OWNER_CHECKER_REF")
		assert.Empty(t, api.requests)
	})
}

func TestValidOwnerCheckerAPIFailures(t *testing.T) {
	tests := map[string]struct {
		ref            string
		status         int
		body           string
		wantErrorParts []string
	}{
		"Repository or ref not found": {
			ref:            "happy-path",
			status:         http.StatusNotFound,
			body:           `{"message":"Not Found"}`,
			wantErrorParts: []string{`org/repo at ref "happy-path"`, "was not found", "Contents read"},
		},
		"Invalid token": {
			status:         http.StatusUnauthorized,
			body:           `{"message":"Bad credentials"}`,
			wantErrorParts: []string{"org/repo", "401 Bad credentials", "GITHUB_ACCESS_TOKEN"},
		},
		"Server error for a ref that is not on GitHub": {
			status:         http.StatusInternalServerError,
			body:           ``,
			wantErrorParts: []string{"org/repo", "500", "must exist on GitHub", "OWNER_CHECKER_REF"},
		},
		"Rate limit": {
			status:         http.StatusForbidden,
			body:           `{"message":"API rate limit exceeded for 1.2.3.4."}`,
			wantErrorParts: []string{"org/repo", "rate limit", "API rate limit exceeded"},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// given
			api := &codeownersErrorsAPI{status: tc.status, body: tc.body}
			ownerCheck := newOwnerCheck(t, check.ValidOwnerConfig{Ref: tc.ref}, api)

			// when
			out, err := ownerCheck.Check(context.Background(), check.Input{
				CodeownersEntries: parseEntries("* @owner-a\n"),
			})

			// then
			require.Error(t, err)
			assert.Empty(t, out.Issues, "API failures must fail the check instead of reporting issues")
			for _, part := range tc.wantErrorParts {
				assert.Contains(t, err.Error(), part)
			}
		})
	}
}
