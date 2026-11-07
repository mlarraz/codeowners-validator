package printer

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mlarraz/codeowners-validator/internal/check"
	"github.com/mlarraz/codeowners-validator/internal/ptr"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGitHubActionsPrinter(t *testing.T) {
	// given
	buff := &bytes.Buffer{}
	restore := overrideWriter(buff)
	defer restore()

	summary := filepath.Join(t.TempDir(), "summary.md")
	require.NoError(t, os.WriteFile(summary, []byte("previous step\n"), 0o600))
	p := NewGitHubActionsPrinter(`D:\a\repo,x\.github\CODEOWNERS`, summary)

	// when
	p.PrintCheckResult("Zed Checker", time.Second, check.Output{}, errors.New("100% broken"))
	p.PrintCheckResult("Foo Checker", time.Second, check.Output{Issues: []check.Issue{
		{Severity: check.Error, LineNo: ptr.Uint64Ptr(3), Message: "Pattern \"/a\" is defined 2 times in lines:\n * 1\n * 3"},
		{Severity: check.Warning, Message: "File-level ```warning```"},
	}}, nil)
	p.PrintCheckResult("Bar Checker", time.Second, check.Output{}, nil)
	p.PrintSummary(3, 2)

	// then
	out := buff.String()
	assert.Contains(t, out, "3 check(s) executed, 2 failure(s)\n"+
		"::error file=D%3A\\a\\repo%2Cx\\.github\\CODEOWNERS,line=3,title=Foo Checker::Pattern \"/a\" is defined 2 times in lines:%0A * 1%0A * 3\n"+
		"::warning file=D%3A\\a\\repo%2Cx\\.github\\CODEOWNERS,title=Foo Checker::File-level ```warning```\n"+
		"::error title=Zed Checker::Internal error: 100%25 broken\n")

	got, err := os.ReadFile(summary)
	require.NoError(t, err)
	assert.Equal(t, "previous step\n"+
		"## CODEOWNERS Validator\n\n"+
		"| Check | Result |\n|---|---|\n"+
		"| Bar Checker | Passed |\n"+
		"| Foo Checker | 1 error, 1 warning |\n"+
		"| Zed Checker | Internal error |\n"+
		"\n3 check(s) executed, 2 failure(s)\n"+
		"\n<details><summary>Foo Checker</summary>\n\n"+
		"````text\n"+
		"[err] line 3: Pattern \"/a\" is defined 2 times in lines:\n * 1\n * 3\n"+
		"[war] File-level ```warning```\n"+
		"````\n"+
		"\n</details>\n"+
		"\n<details><summary>Zed Checker</summary>\n\n"+
		"```text\n"+
		"[Internal Error] 100% broken\n"+
		"```\n"+
		"\n</details>\n", string(got))
}

func TestIssuesTextTruncation(t *testing.T) {
	// given
	line := strings.Repeat("x", 99)
	issues := make([]check.Issue, 2*summaryCheckLimit/len(line))
	for i := range issues {
		issues[i] = check.Issue{Severity: check.Error, Message: line}
	}

	// when
	text := issuesText(checkResult{out: check.Output{Issues: issues}})

	// then
	assert.LessOrEqual(t, len(text), summaryCheckLimit+100)
	assert.True(t, strings.HasSuffix(text, "[err] "+line+"\n... truncated, see the job log for the full output\n"), "the output is cut at a line boundary")
}
