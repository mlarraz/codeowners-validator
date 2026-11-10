package check_test

import (
	"context"
	"testing"

	"github.com/mlarraz/codeowners-validator/internal/check"
	"github.com/mlarraz/codeowners-validator/internal/ptr"
	"github.com/mlarraz/codeowners-validator/pkg/codeowners"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidSyntaxChecker(t *testing.T) {
	tests := map[string]struct {
		codeowners string
		issue      *check.Issue
	}{
		"No owners": {
			codeowners: `*`,
			issue:      nil,
		},
		"Bad username": {
			codeowners: `pkg/github.com/** @-`,
			issue: &check.Issue{
				Severity: check.Warning,
				LineNo:   ptr.Uint64Ptr(1),
				Message:  "Owner '@-' does not look like a GitHub username or team name",
				Key:      "Owner '@-' does not look like a GitHub username or team name: pkg/github.com/** @-",
			},
		},
		"Bad org": {
			codeowners: `* @bad+org`,
			issue: &check.Issue{
				Severity: check.Warning,
				LineNo:   ptr.Uint64Ptr(1),
				Message:  "Owner '@bad+org' does not look like a GitHub username or team name",
				Key:      "Owner '@bad+org' does not look like a GitHub username or team name: * @bad+org",
			},
		},
		"Bad team name on first place": {
			codeowners: `* @org/+not+a+good+name`,
			issue: &check.Issue{
				Severity: check.Warning,
				LineNo:   ptr.Uint64Ptr(1),
				Message:  "Owner '@org/+not+a+good+name' does not look like a GitHub username or team name",
				Key:      "Owner '@org/+not+a+good+name' does not look like a GitHub username or team name: * @org/+not+a+good+name",
			},
		},
		"Bad team name on second place": {
			codeowners: `* @org/hakuna-matata @org/-a-team`,
			issue: &check.Issue{
				Severity: check.Warning,
				LineNo:   ptr.Uint64Ptr(1),
				Message:  "Owner '@org/-a-team' does not look like a GitHub username or team name",
				Key:      "Owner '@org/-a-team' does not look like a GitHub username or team name: * @org/hakuna-matata @org/-a-team",
			},
		},
		"Doesn't look like username, team name, nor email": {
			codeowners: `* something_weird`,
			issue: &check.Issue{
				Severity: check.Error,
				LineNo:   ptr.Uint64Ptr(1),
				Message:  "Owner 'something_weird' does not look like an email",
				Key:      "Owner 'something_weird' does not look like an email: * something_weird",
			},
		},
		"Comment in pattern line": {
			codeowners: `* @org/hakuna-matata # this is allowed`,
		},
	}
	for tn, tc := range tests {
		t.Run(tn, func(t *testing.T) {
			// when
			out, err := check.NewValidSyntax().
				Check(context.Background(), LoadInput(tc.codeowners))

			// then
			require.NoError(t, err)

			assertIssue(t, tc.issue, out.Issues)
		})
	}
}

func TestValidSyntaxZeroValueEntry(t *testing.T) {
	// given
	zeroValueInput := check.Input{
		CodeownersEntries: []codeowners.Entry{
			{
				LineNo:  0,
				Pattern: "",
				Owners:  nil,
			},
		},
	}
	expIssues := []check.Issue{
		{
			LineNo:   ptr.Uint64Ptr(0),
			Severity: check.Error,
			Message:  "Missing pattern",
			Key:      "Missing pattern",
		},
	}

	// when
	out, err := check.NewValidSyntax().
		Check(context.Background(), zeroValueInput)

	// then
	require.NoError(t, err)

	require.Len(t, out.Issues, len(expIssues))
	assert.EqualValues(t, expIssues, out.Issues)
}

func TestValidSyntaxFileSize(t *testing.T) {
	tests := map[string]struct {
		size  int64
		issue *check.Issue
	}{
		"Below the warning threshold": {
			size: 2_516_582,
		},
		"At the warning threshold": {
			size: 2_516_583,
			issue: &check.Issue{
				Severity: check.Warning,
				Message:  "The CODEOWNERS file is 2,516,583 bytes, 80% of GitHub's limit of 3,145,728 bytes (3 MiB). GitHub ignores a file over the limit.",
				Key:      "CODEOWNERS file near the size limit",
			},
		},
		"At the limit, which GitHub still loads": {
			size: 3_145_728,
			issue: &check.Issue{
				Severity: check.Warning,
				Message:  "The CODEOWNERS file is 3,145,728 bytes, 100% of GitHub's limit of 3,145,728 bytes (3 MiB). GitHub ignores a file over the limit.",
				Key:      "CODEOWNERS file near the size limit",
			},
		},
		"Over the limit": {
			size: 3_145_729,
			issue: &check.Issue{
				Severity: check.Error,
				Message:  "The CODEOWNERS file is 3,145,729 bytes, over GitHub's limit of 3,145,728 bytes (3 MiB). GitHub ignores the file and doesn't request reviews from code owners.",
				Key:      "CODEOWNERS file over the size limit",
			},
		},
	}
	for tn, tc := range tests {
		t.Run(tn, func(t *testing.T) {
			// given
			in := LoadInput("* @org/team")
			in.CodeownersSize = tc.size

			// when
			out, err := check.NewValidSyntax().Check(context.Background(), in)

			// then
			require.NoError(t, err)
			assertIssue(t, tc.issue, out.Issues)
		})
	}
}
