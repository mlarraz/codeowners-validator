package check_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/mlarraz/codeowners-validator/internal/check"
	"github.com/mlarraz/codeowners-validator/internal/ptr"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAvoidShadowing(t *testing.T) {
	files := []string{"apps/one/a.txt", "apps/two/b.txt", "build/logs/out.log", "build/main.go", "docs/guide.md"}
	var many strings.Builder
	many.WriteString("/many/ @many\n")
	for i := 1; i <= 12; i++ {
		files = append(files, fmt.Sprintf("many/f%d", i))
		fmt.Fprintf(&many, "/many/f%d @f%d\n", i, i)
	}
	repo := newTestRepo(t, files...)

	tests := map[string]struct {
		codeownersInput string
		expectedIssues  []check.Issue
	}{
		"Should report lines that later lines take precedence over for every file": {
			codeownersInput: "/build/logs/ @logs\n" +
				"/b*/logs @wildcard-logs\n" +
				"/apps/ @apps\n" +
				"/apps/one @one\n" +
				"/apps/two\n" +
				"/docs/guide.md @docs\n" +
				"docs/guide.md @docs-again\n",
			expectedIssues: []check.Issue{
				{
					Severity: check.Error,
					LineNo:   ptr.Uint64Ptr(1),
					Message: `Pattern "/build/logs/" has no effect: later lines take precedence for every file it matches:
            * 2: "/b*/logs"`,
					Key: "/build/logs/",
				},
				{
					Severity: check.Error,
					LineNo:   ptr.Uint64Ptr(3),
					Message: `Pattern "/apps/" has no effect: later lines take precedence for every file it matches:
            * 4: "/apps/one"
            * 5: "/apps/two"`,
					Key: "/apps/",
				},
				{
					Severity: check.Error,
					LineNo:   ptr.Uint64Ptr(6),
					Message: `Pattern "/docs/guide.md" has no effect: later lines take precedence for every file it matches:
            * 7: "docs/guide.md"`,
					Key: "/docs/guide.md",
				},
			},
		},
		"Should report the lines before a catch-all pattern": {
			codeownersInput: "*.md @md\n" +
				"/build/ @build\n" +
				"* @everyone\n",
			expectedIssues: []check.Issue{
				{
					Severity: check.Error,
					LineNo:   ptr.Uint64Ptr(1),
					Message: `Pattern "*.md" has no effect: later lines take precedence for every file it matches:
            * 3: "*"`,
					Key: "*.md",
				},
				{
					Severity: check.Error,
					LineNo:   ptr.Uint64Ptr(2),
					Message: `Pattern "/build/" has no effect: later lines take precedence for every file it matches:
            * 3: "*"`,
					Key: "/build/",
				},
			},
		},
		"Should list at most 10 later lines": {
			codeownersInput: many.String(),
			expectedIssues: []check.Issue{
				{
					Severity: check.Error,
					LineNo:   ptr.Uint64Ptr(1),
					Message: `Pattern "/many/" has no effect: later lines take precedence for every file it matches:
            * 2: "/many/f1"
            * 3: "/many/f2"
            * 4: "/many/f3"
            * 5: "/many/f4"
            * 6: "/many/f5"
            * 7: "/many/f6"
            * 8: "/many/f7"
            * 9: "/many/f8"
            * 10: "/many/f9"
            * 11: "/many/f10"
            * and 2 more`,
					Key: "/many/",
				},
			},
		},
		"Should not report lines that own some files or don't match any": {
			codeownersInput: "* @everyone\n" +
				"/build/ @build\n" +
				"/build/logs/ @logs\n" +
				"/missing/ @nobody\n" +
				"*.md @md\n",
			expectedIssues: nil,
		},
	}

	for tn, tc := range tests {
		t.Run(tn, func(t *testing.T) {
			// given
			sut := check.NewAvoidShadowing()
			in := LoadInput(tc.codeownersInput)
			in.Repo = repo

			// when
			out, err := sut.Check(context.TODO(), in)

			// then
			require.NoError(t, err)
			assert.Equal(t, tc.expectedIssues, out.Issues)
		})
	}
}
