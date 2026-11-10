package printer

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mlarraz/codeowners-validator/internal/check"
)

// summaryCheckLimit is the maximum number of bytes of issues that the job summary shows for a
// check. GitHub rejects job summaries over 1 MiB, so long outputs, such as thousands of not owned
// files, are truncated; the job log still has every issue.
const summaryCheckLimit = 100 << 10

// GitHubActionsPrinter prints the check results like TTYPrinter, and reports them to GitHub Actions:
// every issue becomes an annotation on the CODEOWNERS file, and the results are added to the job
// summary.
type GitHubActionsPrinter struct {
	tty TTYPrinter
	// file is the absolute path of the CODEOWNERS file. The runner turns it into the path in the
	// repository.
	file string
	// summaryPath is the path of the job summary file, from GITHUB_STEP_SUMMARY. The summary isn't
	// written if it's empty.
	summaryPath string

	m       sync.Mutex
	results []checkResult
}

type checkResult struct {
	name string
	out  check.Output
	err  error
}

// NewGitHubActionsPrinter returns a printer that reports issues as annotations on the given
// CODEOWNERS file and writes the job summary to summaryPath.
func NewGitHubActionsPrinter(file, summaryPath string) *GitHubActionsPrinter {
	return &GitHubActionsPrinter{file: file, summaryPath: summaryPath}
}

func (p *GitHubActionsPrinter) PrintCheckResult(checkName string, duration time.Duration, checkOut check.Output, checkErr error) {
	p.tty.PrintCheckResult(checkName, duration, checkOut, checkErr)

	p.m.Lock()
	defer p.m.Unlock()
	p.results = append(p.results, checkResult{name: checkName, out: checkOut, err: checkErr})
}

// PrintSummary prints the summary, and then the annotations and the job summary for all checks,
// sorted by check name.
func (p *GitHubActionsPrinter) PrintSummary(allCheck, failedChecks int) {
	p.tty.PrintSummary(allCheck, failedChecks)

	p.m.Lock()
	defer p.m.Unlock()
	sort.SliceStable(p.results, func(i, j int) bool { return p.results[i].name < p.results[j].name })

	for _, r := range p.results {
		p.printAnnotations(r)
	}

	if p.summaryPath == "" {
		return
	}
	if err := p.writeJobSummary(allCheck, failedChecks); err != nil {
		fmt.Fprintf(writer, "::warning::%s\n", escapeData(fmt.Sprintf("Unable to write the job summary: %s", err)))
	}
}

// printAnnotations prints a workflow command for every issue of the check, see
// https://docs.github.com/en/actions/reference/workflows-and-actions/workflow-commands
func (p *GitHubActionsPrinter) printAnnotations(r checkResult) {
	for _, i := range r.out.Issues {
		props := []string{"file=" + escapeProperty(p.file)}
		if i.LineNo != nil {
			props = append(props, fmt.Sprintf("line=%d", *i.LineNo))
		}
		props = append(props, "title="+escapeProperty(r.name))
		fmt.Fprintf(writer, "::%s %s::%s\n", annotationLevel(i.Severity), strings.Join(props, ","), escapeData(i.Message))
	}

	if r.err != nil {
		fmt.Fprintf(writer, "::error title=%s::%s\n", escapeProperty(r.name), escapeData(fmt.Sprintf("Internal error: %s", r.err)))
	}
}

func annotationLevel(s check.SeverityType) string {
	switch s {
	case check.Info:
		return "notice"
	case check.Warning:
		return "warning"
	default:
		return "error"
	}
}

func (p *GitHubActionsPrinter) writeJobSummary(allCheck, failedChecks int) error {
	f, err := os.OpenFile(p.summaryPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}

	writeErr := writeJobSummary(f, p.results, allCheck, failedChecks)
	if err := f.Close(); err != nil && writeErr == nil {
		return err
	}
	return writeErr
}

func writeJobSummary(w io.Writer, results []checkResult, allCheck, failedChecks int) error {
	var b strings.Builder

	b.WriteString("## CODEOWNERS Validator\n\n")
	b.WriteString("| Check | Result |\n|---|---|\n")
	for _, r := range results {
		fmt.Fprintf(&b, "| %s | %s |\n", r.name, resultText(r))
	}

	failures := "no"
	if failedChecks > 0 {
		failures = fmt.Sprintf("%d", failedChecks)
	}
	fmt.Fprintf(&b, "\n%d check(s) executed, %s failure(s)\n", allCheck, failures)

	for _, r := range results {
		if len(r.out.Issues) == 0 && r.err == nil {
			continue
		}
		fmt.Fprintf(&b, "\n<details><summary>%s</summary>\n\n", r.name)
		writeCodeBlock(&b, issuesText(r))
		b.WriteString("\n</details>\n")
	}

	_, err := io.WriteString(w, b.String())
	return err
}

// resultText describes the outcome of a check in a table cell.
func resultText(r checkResult) string {
	if r.err != nil {
		return "Internal error"
	}

	var errs, warnings, infos int
	for _, i := range r.out.Issues {
		switch i.Severity {
		case check.Info:
			infos++
		case check.Warning:
			warnings++
		default:
			errs++
		}
	}

	var parts []string
	if errs > 0 {
		parts = append(parts, plural(errs, "error"))
	}
	if warnings > 0 {
		parts = append(parts, plural(warnings, "warning"))
	}
	if infos > 0 {
		parts = append(parts, fmt.Sprintf("%d info", infos))
	}
	if len(parts) == 0 {
		return "Passed"
	}
	return strings.Join(parts, ", ")
}

func plural(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// issuesText lists the issues of the check the way TTYPrinter does, truncated to
// summaryCheckLimit bytes at a line boundary.
func issuesText(r checkResult) string {
	var b strings.Builder
	for _, i := range r.out.Issues {
		fmt.Fprintf(&b, "[%s]", strings.ToLower(i.Severity.String()[:3]))
		if i.LineNo != nil {
			fmt.Fprintf(&b, " line %d:", *i.LineNo)
		}
		fmt.Fprintf(&b, " %s\n", i.Message)
	}
	if r.err != nil {
		fmt.Fprintf(&b, "[Internal Error] %s\n", r.err)
	}

	text := b.String()
	if len(text) <= summaryCheckLimit {
		return text
	}
	cut := strings.LastIndexByte(text[:summaryCheckLimit], '\n') + 1
	return text[:cut] + "... truncated, see the job log for the full output\n"
}

// writeCodeBlock writes text as a fenced code block, with a fence longer than any run of
// backticks in the text so that the text can't end the block.
func writeCodeBlock(b *strings.Builder, text string) {
	longest, run := 0, 0
	for _, c := range text {
		if c == '`' {
			run++
			if run > longest {
				longest = run
			}
		} else {
			run = 0
		}
	}
	fence := strings.Repeat("`", max(3, longest+1))

	b.WriteString(fence + "text\n")
	b.WriteString(text)
	if !strings.HasSuffix(text, "\n") {
		b.WriteString("\n")
	}
	b.WriteString(fence + "\n")
}

// escapeData escapes the message of a workflow command.
func escapeData(s string) string {
	return strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A").Replace(s)
}

// escapeProperty escapes a property value of a workflow command.
func escapeProperty(s string) string {
	return strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A", ":", "%3A", ",", "%2C").Replace(s)
}
