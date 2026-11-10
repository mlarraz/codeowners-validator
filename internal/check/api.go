package check

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/mlarraz/codeowners-validator/internal/ptr"
	"github.com/mlarraz/codeowners-validator/internal/repo"
	"github.com/mlarraz/codeowners-validator/pkg/codeowners"
)

type (
	// Checker allows to execute check in a generic way
	Checker interface {
		Check(ctx context.Context, in Input) (Output, error)
		Name() string
	}

	Issue struct {
		Severity SeverityType // enum // default error
		LineNo   *uint64
		Message  string
	}

	Input struct {
		Repo              *repo.Repo
		CodeownersEntries []codeowners.Entry
		// CodeownersSize is the size of the CODEOWNERS file in bytes.
		CodeownersSize int64
	}

	Output struct {
		Issues []Issue
	}

	OutputBuilder struct {
		mux    sync.Mutex
		issues []Issue
	}
)

type ReportIssueOpt func(*Issue)

func WithSeverity(s SeverityType) ReportIssueOpt {
	return func(i *Issue) {
		i.Severity = s
	}
}

// WithLineNo sets the line in the CODEOWNERS file that the issue refers to.
func WithLineNo(line uint64) ReportIssueOpt {
	return func(i *Issue) {
		i.LineNo = ptr.Uint64Ptr(line)
	}
}

func WithEntry(e codeowners.Entry) ReportIssueOpt {
	return func(i *Issue) {
		i.LineNo = ptr.Uint64Ptr(e.LineNo)
	}
}

func (bldr *OutputBuilder) ReportIssue(msg string, opts ...ReportIssueOpt) *OutputBuilder {
	if bldr == nil { // TODO: error?
		return nil
	}

	i := Issue{
		Severity: Error,
		Message:  msg,
	}

	for _, opt := range opts {
		opt(&i)
	}

	bldr.mux.Lock()
	defer bldr.mux.Unlock()
	bldr.issues = append(bldr.issues, i)

	return bldr
}

func (bldr *OutputBuilder) Output() Output {
	if bldr == nil {
		return Output{}
	}
	return Output{Issues: bldr.issues}
}

// SeverityType is the severity of an issue. Lower values are more severe.
type SeverityType int

const (
	Error SeverityType = iota + 1
	Warning
	// Info issues are reported but never fail the validation, whatever CHECK_FAILURE_LEVEL is.
	Info
)

func (s SeverityType) String() string {
	switch s {
	case Error:
		return "Error"
	case Warning:
		return "Warning"
	case Info:
		return "Info"
	default:
		return ""
	}
}

// Unmarshal provides custom parsing of severity type.
// Implements envconfig.Unmarshal interface.
func (s *SeverityType) Unmarshal(in string) error {
	switch strings.ToLower(in) {
	case "error", "err":
		*s = Error
	case "warning", "warn":
		*s = Warning
	default:
		return fmt.Errorf("not a valid severity type: %q", in)
	}

	return nil
}

// ParseLevel parses the level of a check, the most severe level at which the check reports its
// issues. Possible values are warning and info.
func ParseLevel(in string) (SeverityType, error) {
	switch strings.ToLower(in) {
	case "warning", "warn":
		return Warning, nil
	case "info":
		return Info, nil
	default:
		return 0, fmt.Errorf("not a valid check level: %q, possible values are warning and info", in)
	}
}

// WithLevel returns a checker that runs c and reports its issues at the given level at most: with
// Info, all of its issues are reported as Info issues and never fail the validation. Errors that
// stop the check from running are still reported as failures.
func WithLevel(c Checker, level SeverityType) Checker {
	return leveledChecker{Checker: c, level: level}
}

type leveledChecker struct {
	Checker
	level SeverityType
}

func (c leveledChecker) Check(ctx context.Context, in Input) (Output, error) {
	out, err := c.Checker.Check(ctx, in)
	for i := range out.Issues {
		if out.Issues[i].Severity < c.level {
			out.Issues[i].Severity = c.level
		}
	}
	return out, err
}
