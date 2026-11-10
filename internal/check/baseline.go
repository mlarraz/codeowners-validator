package check

import (
	"context"
	"fmt"
)

// WithBaseline returns a checker that runs c and reports the issues whose keys are listed in
// known as Info issues, marked as baselined, so that they don't fail the validation. A listed key
// that no issue has anymore is reported as a warning, so that the baseline only shrinks: the
// entry has to be removed once the issue is fixed.
func WithBaseline(c Checker, known []string) Checker {
	return baselineChecker{Checker: c, known: known}
}

type baselineChecker struct {
	Checker
	known []string
}

func (c baselineChecker) Check(ctx context.Context, in Input) (Output, error) {
	out, err := c.Checker.Check(ctx, in)
	if err != nil {
		// The issues of a check that didn't finish don't say which entries are stale.
		return out, err
	}

	seen := make(map[string]bool, len(c.known))
	for _, k := range c.known {
		seen[k] = false
	}
	for i := range out.Issues {
		issue := &out.Issues[i]
		if _, ok := seen[issue.Key]; !ok || issue.Key == "" {
			continue
		}
		seen[issue.Key] = true
		issue.Severity = Info
		issue.Baselined = true
	}

	for _, k := range c.known {
		if seen[k] {
			continue
		}
		seen[k] = true // a key listed twice is reported once
		out.Issues = append(out.Issues, Issue{
			Severity: Warning,
			Message:  fmt.Sprintf("Baseline entry %q doesn't match any issue anymore: remove it from the baseline", k),
		})
	}

	return out, nil
}
