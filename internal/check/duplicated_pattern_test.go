package check_test

import (
	"context"
	"testing"

	"github.com/mlarraz/codeowners-validator/internal/check"
	"github.com/mlarraz/codeowners-validator/internal/ptr"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDuplicatedPattern(t *testing.T) {
	tests := map[string]struct {
		codeownersInput string
		expectedIssues  []check.Issue
	}{
		"Should report info about duplicated entries": {
			codeownersInput: `
					*       @global-owner1 @global-owner2

					/build/logs/ @doctocat
					/build/logs/ @doctocat

					/script @script-owner
					/script m.t@g.com
			`,
			expectedIssues: []check.Issue{
				{
					Severity: check.Error,
					LineNo:   ptr.Uint64Ptr(5),
					Message: `Pattern "/build/logs/" is defined 2 times in lines:
            * 4: with owners: [@doctocat]
            * 5: with owners: [@doctocat]`,
					Key: "/build/logs/",
				},
				{
					Severity: check.Error,
					LineNo:   ptr.Uint64Ptr(8),
					Message: `Pattern "/script" is defined 2 times in lines:
            * 7: with owners: [@script-owner]
            * 8: with owners: [m.t@g.com]`,
					Key: "/script",
				},
			},
		},
		"Should report patterns that are anchored at the root with and without a leading slash": {
			codeownersInput: `
					docs/api @docs
					/docs/api @api
					**/vendor/ @vendor
					/**/vendor/ @vendor-again
			`,
			expectedIssues: []check.Issue{
				{
					Severity: check.Error,
					LineNo:   ptr.Uint64Ptr(3),
					Message: `Pattern "/docs/api" is defined 2 times in lines:
            * 2: "docs/api" with owners: [@docs]
            * 3: "/docs/api" with owners: [@api]`,
					Key: "/docs/api",
				},
				{
					Severity: check.Error,
					LineNo:   ptr.Uint64Ptr(5),
					Message: `Pattern "/**/vendor/" is defined 2 times in lines:
            * 4: "**/vendor/" with owners: [@vendor]
            * 5: "/**/vendor/" with owners: [@vendor-again]`,
					Key: "/**/vendor/",
				},
			},
		},
		"Should not report patterns that only a trailing slash or a leading slash differentiates": {
			codeownersInput: `
					docs @docs
					/docs @root-docs
					docs/ @docs-dirs
					/docs/ @root-docs-dir
			`,
			expectedIssues: nil,
		},
		"Should not report any issues with correct CODEOWNERS file": {
			codeownersInput: FixtureValidCODEOWNERS,
			expectedIssues:  nil,
		},
	}

	for tn, tc := range tests {
		t.Run(tn, func(t *testing.T) {
			// given
			sut := check.NewDuplicatedPattern()

			// when
			out, err := sut.Check(context.TODO(), LoadInput(tc.codeownersInput))

			// then
			require.NoError(t, err)
			assert.ElementsMatch(t, tc.expectedIssues, out.Issues)
		})
	}
}
