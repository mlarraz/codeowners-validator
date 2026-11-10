package check_test

import (
	"context"
	"errors"
	"testing"

	"github.com/mlarraz/codeowners-validator/internal/check"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAPIBuilder(t *testing.T) {
	var bldr *check.OutputBuilder

	t.Run("Does not panic on ReportIssue when builder is nil", func(t *testing.T) {
		assert.NotPanics(t, func() {
			issue := bldr.ReportIssue("test")
			assert.Nil(t, issue)
		})
	})

	t.Run("Does not panic on Output when builder is nil", func(t *testing.T) {
		assert.NotPanics(t, func() {
			out := bldr.Output()
			assert.Empty(t, out)
		})
	})
}

type fakeChecker struct {
	out check.Output
	err error
}

func (f fakeChecker) Check(context.Context, check.Input) (check.Output, error) {
	// Return a copy so that a checker that changes the issues doesn't change the fixture.
	return check.Output{Issues: append([]check.Issue(nil), f.out.Issues...)}, f.err
}

func (fakeChecker) Name() string { return "Fake Checker" }

func TestWithLevel(t *testing.T) {
	checkErr := errors.New("broken")
	inner := fakeChecker{
		out: check.Output{Issues: []check.Issue{
			{Severity: check.Error, Message: "error"},
			{Severity: check.Warning, Message: "warning"},
			{Severity: check.Info, Message: "info"},
		}},
		err: checkErr,
	}

	tests := []struct {
		level check.SeverityType
		want  []check.SeverityType
	}{
		{level: check.Warning, want: []check.SeverityType{check.Warning, check.Warning, check.Info}},
		{level: check.Info, want: []check.SeverityType{check.Info, check.Info, check.Info}},
	}
	for _, tc := range tests {
		t.Run(tc.level.String(), func(t *testing.T) {
			// when
			c := check.WithLevel(inner, tc.level)
			out, err := c.Check(context.Background(), check.Input{})

			// then
			assert.Equal(t, "Fake Checker", c.Name())
			assert.ErrorIs(t, err, checkErr, "errors that stop the check are kept")
			var got []check.SeverityType
			for _, i := range out.Issues {
				got = append(got, i.Severity)
			}
			assert.Equal(t, tc.want, got, "issues are lowered to the level, never raised")
		})
	}
}

func TestParseLevel(t *testing.T) {
	for in, want := range map[string]check.SeverityType{"warning": check.Warning, "WARN": check.Warning, "info": check.Info} {
		got, err := check.ParseLevel(in)
		require.NoError(t, err, in)
		assert.Equal(t, want, got, in)
	}

	// error is the level that checks already report at, so it isn't a level to lower them to.
	for _, in := range []string{"error", "", "notice"} {
		_, err := check.ParseLevel(in)
		assert.Error(t, err, in)
	}
}
