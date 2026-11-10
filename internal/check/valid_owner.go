package check

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/go-github/v93/github"
	"github.com/pkg/errors"

	"github.com/mlarraz/codeowners-validator/internal/ctxutil"
	"github.com/mlarraz/codeowners-validator/internal/repo"
)

type ValidOwnerConfig struct {
	// Repository represents the GitHub repository whose CODEOWNERS file is validated.
	// It is in form 'owner/repository'.
	Repository string
	// Ref is a branch, tag or commit SHA that determines which version of the CODEOWNERS
	// file is validated. Defaults to the commit checked out in the validated repository.
	// If the validated directory isn't a Git repository, the repository's default branch is used.
	Ref string `envconfig:"optional"`
	// IgnoredOwners contains a list of owners that should not be validated.
	// Defaults to @ghost.
	// More info about the @ghost user: https://docs.github.com/en/free-pro-team@latest/github/setting-up-and-managing-your-github-user-account/deleting-your-user-account
	// Tip on how @ghost can be used: https://github.community/t5/How-to-use-Git-and-GitHub/CODEOWNERS-file-with-a-NOT-file-type-condition/m-p/31013/highlight/true#M8523
	IgnoredOwners []string `envconfig:"default=@ghost"`
	// AllowUnownedPatterns specifies whether CODEOWNERS may have unowned files. For example:
	//
	//  /infra/oncall-rotator/                    @sre-team
	//  /infra/oncall-rotator/oncall-config.yml
	//
	//  The `/infra/oncall-rotator/oncall-config.yml` this file is not owned by anyone.
	AllowUnownedPatterns bool `envconfig:"default=true"`
	// OwnersMustBeTeams specifies whether owners must be teams.
	OwnersMustBeTeams bool `envconfig:"default=false"`
}

// ValidOwner validates the owners defined in the CODEOWNERS file.
//
// GitHub resolves CODEOWNERS owners when it requests reviews, so the check reports the errors
// that GitHub itself finds in the CODEOWNERS file at the validated ref. This covers owners that
// don't exist, have no access to the repository, are teams from another organization, and so on.
type ValidOwner struct {
	ghClient             *github.Client
	owner                string
	repo                 string
	ref                  string
	ignoredOwners        map[string]struct{}
	allowUnownedPatterns bool
	ownersMustBeTeams    bool
}

// NewValidOwner returns new instance of the ValidOwner
func NewValidOwner(cfg ValidOwnerConfig, ghClient *github.Client) (*ValidOwner, error) {
	split := strings.Split(cfg.Repository, "/")
	if len(split) != 2 || split[0] == "" || split[1] == "" {
		return nil, errors.Errorf("Wrong repository name. Expected pattern 'owner/repository', got '%s'", cfg.Repository)
	}

	ignoredOwners := make(map[string]struct{}, len(cfg.IgnoredOwners))
	for _, owner := range cfg.IgnoredOwners {
		ignoredOwners[strings.ToLower(owner)] = struct{}{}
	}

	return &ValidOwner{
		ghClient:             ghClient,
		owner:                split[0],
		repo:                 split[1],
		ref:                  cfg.Ref,
		ignoredOwners:        ignoredOwners,
		allowUnownedPatterns: cfg.AllowUnownedPatterns,
		ownersMustBeTeams:    cfg.OwnersMustBeTeams,
	}, nil
}

// Check reports the CODEOWNERS errors that GitHub identifies for the validated repository.
func (v *ValidOwner) Check(ctx context.Context, in Input) (Output, error) {
	if ctxutil.ShouldExit(ctx) {
		return Output{}, ctx.Err()
	}

	var bldr OutputBuilder

	for _, entry := range in.CodeownersEntries {
		if len(entry.Owners) == 0 && !v.allowUnownedPatterns {
			msg := "Missing owner, at least one owner is required"
			bldr.ReportIssue(msg, WithEntry(entry), WithSeverity(Warning), WithKey(entryKey(msg, entry)))
			continue
		}

		if v.ownersMustBeTeams {
			for _, owner := range entry.Owners {
				if v.isIgnoredOwner(owner) {
					continue
				}
				if !isGitHubTeam(owner) {
					msg := fmt.Sprintf("Only team owners allowed and %q is not a team", owner)
					bldr.ReportIssue(msg, WithEntry(entry), WithKey(entryKey(msg, entry)))
				}
			}
		}
	}

	ref, err := v.resolveRef(ctx, in)
	if err != nil {
		return Output{}, err
	}

	errs, err := v.codeownersErrors(ctx, ref)
	if err != nil {
		return Output{}, err
	}

	for _, apiErr := range errs {
		owner := ownerToken(apiErr)
		if v.isIgnoredOwner(owner) {
			continue
		}

		// GitHub numbers lines from 1; an error without a line is reported for the whole file.
		msg := codeownersErrorMessage(apiErr, owner)
		opts := []ReportIssueOpt{WithKey(lineKey(msg, apiErr.GetSource()))}
		if line := apiErr.GetLine(); line > 0 {
			opts = append(opts, WithLineNo(uint64(line)))
		}
		bldr.ReportIssue(msg, opts...)
	}

	return bldr.Output(), nil
}

// resolveRef returns the ref for which the CODEOWNERS file is validated. An empty ref means
// GitHub's default branch for the repository.
func (v *ValidOwner) resolveRef(ctx context.Context, in Input) (string, error) {
	if v.ref != "" {
		return v.ref, nil
	}
	if in.Repo == nil {
		return "", nil
	}

	head, err := in.Repo.Head(ctx)
	switch {
	case err == nil:
		return head, nil
	case errors.Is(err, repo.ErrNotRepository):
		return "", nil
	default:
		return "", errors.Wrapf(err, "while reading the checked-out commit of %s; set OWNER_CHECKER_REF to a branch, tag or commit", in.Repo.Dir())
	}
}

func (v *ValidOwner) codeownersErrors(ctx context.Context, ref string) ([]*github.CodeownersError, error) {
	result, _, err := v.ghClient.Repositories.GetCodeownersErrors(ctx, v.owner, v.repo, &github.GetCodeownersErrorsOptions{Ref: ref})
	if err != nil {
		return nil, v.apiError(err, ref)
	}
	return result.GetErrors(), nil
}

func (v *ValidOwner) apiError(err error, ref string) error {
	where := v.owner + "/" + v.repo
	if ref != "" {
		where += " at ref " + fmt.Sprintf("%q", ref)
	}

	var errResp *github.ErrorResponse
	var rateLimitErr *github.RateLimitError
	var abuseRateLimitErr *github.AbuseRateLimitError
	switch {
	case errors.As(err, &rateLimitErr):
		return fmt.Errorf("GitHub rate limit reached while listing CODEOWNERS errors for %s: %s", where, rateLimitErr.Message)
	case errors.As(err, &abuseRateLimitErr):
		return fmt.Errorf("GitHub rate limit reached while listing CODEOWNERS errors for %s: %s", where, abuseRateLimitErr.Message)
	case errors.As(err, &errResp):
		switch errResp.Response.StatusCode {
		case http.StatusNotFound:
			return fmt.Errorf("repository %s, its ref or CODEOWNERS file was not found. Check OWNER_CHECKER_REPOSITORY and OWNER_CHECKER_REF, and make sure the credentials can read the repository: fine-grained tokens and GitHub App installation tokens need the Contents read permission, classic tokens need the repo scope for private repositories", where)
		case http.StatusUnauthorized:
			return fmt.Errorf("GitHub rejected the credentials while listing CODEOWNERS errors for %s: 401 Bad credentials. Check GITHUB_ACCESS_TOKEN", where)
		case http.StatusInternalServerError:
			return fmt.Errorf("GitHub returned 500 while listing CODEOWNERS errors for %s. The ref must exist on GitHub: push it or set OWNER_CHECKER_REF to a branch, tag or commit that does", where)
		default:
			return fmt.Errorf("GitHub returned %s while listing CODEOWNERS errors for %s: %s", errResp.Response.Status, where, strings.TrimSpace(errResp.Message))
		}
	default:
		return fmt.Errorf("while listing CODEOWNERS errors for %s: %w", where, err)
	}
}

// ownerToken returns the owner that the error points at, based on the error's column in the
// erroneous CODEOWNERS line.
func ownerToken(apiErr *github.CodeownersError) string {
	source := apiErr.GetSource()
	start := apiErr.GetColumn() - 1
	if start < 0 || start >= len(source) {
		return ""
	}

	tokens := strings.Fields(source[start:])
	if len(tokens) == 0 {
		return ""
	}
	return tokens[0]
}

func codeownersErrorMessage(apiErr *github.CodeownersError, owner string) string {
	kind := apiErr.GetKind()
	if suggestion := apiErr.GetSuggestion(); suggestion != "" {
		return fmt.Sprintf("%s: %s", kind, suggestion)
	}
	if owner == "" {
		return kind
	}
	return fmt.Sprintf("%s: %s", kind, owner)
}

func isGitHubTeam(s string) bool {
	hasPrefix := strings.HasPrefix(s, "@")
	containsSlash := strings.Contains(s, "/")
	split := strings.SplitN(s, "/", 3) // 3 is enough to confirm that is invalid + will not overflow the buffer
	return hasPrefix && containsSlash && len(split) == 2 && split[1] != ""
}

func (v *ValidOwner) isIgnoredOwner(name string) bool {
	_, found := v.ignoredOwners[strings.ToLower(name)]
	return found
}

// Name returns human-readable name of the validator
func (ValidOwner) Name() string {
	return "Valid Owner Checker"
}
