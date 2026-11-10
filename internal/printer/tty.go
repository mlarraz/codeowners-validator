package printer

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/fatih/color"
	"github.com/mlarraz/codeowners-validator/internal/check"
)

// writer used for test purpose
var writer io.Writer = os.Stdout

type TTYPrinter struct {
	m sync.RWMutex
}

func (tty *TTYPrinter) PrintCheckResult(checkName string, duration time.Duration, checkOut check.Output, checkErr error) {
	tty.m.Lock()
	defer tty.m.Unlock()

	header := color.New(color.Bold).FprintfFunc()
	issueBody := color.New(color.FgWhite).FprintfFunc()
	okCheck := color.New(color.FgGreen).FprintlnFunc()
	errCheck := color.New(color.FgRed).FprintfFunc()

	header(writer, "==> Executing %s (%v)\n", checkName, duration)
	for _, i := range checkOut.Issues {
		issueSeverity := tty.severityPrintfFunc(i.Severity)

		issueSeverity(writer, "    [%s]", strings.ToLower(i.Severity.String()[:3]))
		issueBody(writer, "%s", issueLocation(i))
		issueBody(writer, " %s\n", i.Message)
	}

	switch {
	case checkErr == nil && len(checkOut.Issues) == 0:
		okCheck(writer, "    Check OK")
	case checkErr != nil:
		errCheck(writer, "    [Internal Error]")
		issueBody(writer, " %s\n", checkErr)
	}
}

// issueLocation returns the line of the issue, and whether it's in the baseline, followed by a
// colon, or "" if there's neither.
func issueLocation(i check.Issue) string {
	var s string
	if i.LineNo != nil {
		s += fmt.Sprintf(" line %d", *i.LineNo)
	}
	if i.Baselined {
		s += " (in baseline)"
	}
	if s == "" {
		return ""
	}
	return s + ":"
}

func (*TTYPrinter) severityPrintfFunc(severity check.SeverityType) func(w io.Writer, format string, a ...interface{}) {
	p := color.New()
	switch severity {
	case check.Info:
		p.Add(color.FgCyan)
	case check.Warning:
		p.Add(color.FgYellow)
	case check.Error:
		p.Add(color.FgRed)
	}

	return p.FprintfFunc()
}

func (*TTYPrinter) PrintSummary(allCheck, failedChecks int) {
	failures := "no"
	if failedChecks > 0 {
		failures = fmt.Sprintf("%d", failedChecks)
	}
	fmt.Fprintf(writer, "\n%d check(s) executed, %s failure(s)\n", allCheck, failures)
}
