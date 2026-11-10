package check_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mlarraz/codeowners-validator/internal/check"
)

func TestWithBaseline(t *testing.T) {
	inner := fakeChecker{out: check.Output{Issues: []check.Issue{
		{Severity: check.Error, Message: "known", Key: "/a"},
		{Severity: check.Warning, Message: "known warning", Key: "/b"},
		{Severity: check.Error, Message: "new", Key: "/c"},
		{Severity: check.Error, Message: "without key"},
	}}}

	// when
	out, err := check.WithBaseline(inner, []string{"/a", "/b", "/fixed", "/fixed", ""}).Check(context.Background(), check.Input{})

	// then
	require.NoError(t, err)
	assert.Equal(t, []check.Issue{
		{Severity: check.Info, Message: "known", Key: "/a", Baselined: true},
		{Severity: check.Info, Message: "known warning", Key: "/b", Baselined: true},
		{Severity: check.Error, Message: "new", Key: "/c"},
		{Severity: check.Error, Message: "without key"},
		{Severity: check.Warning, Message: `Baseline entry "/fixed" doesn't match any issue anymore: remove it from the baseline`},
		{Severity: check.Warning, Message: `Baseline entry "" doesn't match any issue anymore: remove it from the baseline`},
	}, out.Issues, "an issue without a key never matches; a stale entry listed twice is reported once")
}

func TestWithBaselineDoesNotReportStaleEntriesWhenTheCheckFails(t *testing.T) {
	checkErr := errors.New("broken")
	inner := fakeChecker{err: checkErr}

	// when
	out, err := check.WithBaseline(inner, []string{"/a"}).Check(context.Background(), check.Input{})

	// then
	assert.ErrorIs(t, err, checkErr)
	assert.Empty(t, out.Issues)
}

func TestWithLevelLowersStaleBaselineEntries(t *testing.T) {
	// given: the order that load.Checks wraps checks in
	c := check.WithLevel(check.WithBaseline(fakeChecker{}, []string{"/fixed"}), check.Info)

	// when
	out, err := c.Check(context.Background(), check.Input{})

	// then
	require.NoError(t, err)
	require.Len(t, out.Issues, 1)
	assert.Equal(t, check.Info, out.Issues[0].Severity, "a check at the info level never fails, not even for stale entries")
}
