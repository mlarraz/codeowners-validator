package runner_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	"github.com/mlarraz/codeowners-validator/internal/check"
	"github.com/mlarraz/codeowners-validator/internal/runner"
)

type fakeChecker struct {
	name     string
	severity check.SeverityType
	err      error
}

func (f fakeChecker) Check(context.Context, check.Input) (check.Output, error) {
	if f.severity == 0 {
		return check.Output{}, f.err
	}
	return check.Output{Issues: []check.Issue{{Severity: f.severity, Message: "issue"}}}, f.err
}

func (f fakeChecker) Name() string { return f.name }

type summaryPrinter struct {
	failedChecks int
}

func (*summaryPrinter) PrintCheckResult(string, time.Duration, check.Output, error) {}

func (p *summaryPrinter) PrintSummary(_, failedChecks int) { p.failedChecks = failedChecks }

func TestCheckRunnerFailures(t *testing.T) {
	tests := []struct {
		name         string
		failureLevel check.SeverityType
		checks       []check.Checker
		wantFailed   int
		wantExit     bool
	}{
		{
			name:         "info issues never fail",
			failureLevel: check.Warning,
			checks:       []check.Checker{fakeChecker{name: "a", severity: check.Info}},
		},
		{
			name:         "warnings fail at the warning level",
			failureLevel: check.Warning,
			checks:       []check.Checker{fakeChecker{name: "a", severity: check.Warning}, fakeChecker{name: "b", severity: check.Info}},
			wantFailed:   1,
			wantExit:     true,
		},
		{
			name:         "warnings don't fail at the error level",
			failureLevel: check.Error,
			checks:       []check.Checker{fakeChecker{name: "a", severity: check.Warning}},
		},
		{
			name:         "errors fail",
			failureLevel: check.Error,
			checks:       []check.Checker{fakeChecker{name: "a", severity: check.Error}, fakeChecker{name: "b"}},
			wantFailed:   1,
			wantExit:     true,
		},
		{
			name:         "a check that can't run fails, whatever its level",
			failureLevel: check.Warning,
			checks:       []check.Checker{check.WithLevel(fakeChecker{name: "a", err: errors.New("broken")}, check.Info)},
			wantFailed:   1,
			wantExit:     true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// given
			p := &summaryPrinter{}
			r := runner.NewCheckRunner(logrus.New(), check.Input{}, tc.failureLevel, p, tc.checks...)

			// when
			r.Run(context.Background())

			// then
			assert.Equal(t, tc.wantFailed, p.failedChecks, "failed checks in the summary")
			assert.Equal(t, tc.wantExit, r.ShouldExitWithCheckFailure(), "exit with the check failure code")
		})
	}
}
