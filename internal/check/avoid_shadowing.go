package check

import (
	"context"
	"fmt"
	"strings"

	"github.com/mlarraz/codeowners-validator/internal/ctxutil"

	"github.com/pkg/errors"
)

// maxListedShadowingLines is the number of later lines that an issue lists.
const maxListedShadowingLines = 10

// AvoidShadowing reports the CODEOWNERS lines that have no effect because later lines take
// precedence for every file they match. GitHub uses the last line that matches a file.
type AvoidShadowing struct{}

func NewAvoidShadowing() *AvoidShadowing {
	return &AvoidShadowing{}
}

func (c *AvoidShadowing) Check(ctx context.Context, in Input) (Output, error) {
	if ctxutil.ShouldExit(ctx) {
		return Output{}, ctx.Err()
	}

	files, err := in.Repo.Files(ctx)
	if err != nil {
		return Output{}, errors.Wrap(err, "while listing the files of the repository")
	}

	matcher := newPathMatcher(files, in.CodeownersEntries)
	shadowed, err := matcher.shadowed(ctx)
	if err != nil {
		return Output{}, errors.Wrap(err, "while looking for lines that don't own any files")
	}

	var bldr OutputBuilder
	for _, s := range shadowed {
		msg := fmt.Sprintf("Pattern %q has no effect: later lines take precedence for every file it matches:\n%s", s.rule.entry.Pattern, c.listFormatFunc(matcher, s.by))
		bldr.ReportIssue(msg, WithEntry(s.rule.entry))
	}

	return bldr.Output(), nil
}

// listFormatFunc lists the given rules as a bullet point list.
func (c *AvoidShadowing) listFormatFunc(m *pathMatcher, rules []int) string {
	listed := rules
	if len(listed) > maxListedShadowingLines {
		listed = listed[:maxListedShadowingLines]
	}

	points := make([]string, 0, len(listed)+1)
	for _, i := range listed {
		entry := m.rules[i].entry
		points = append(points, fmt.Sprintf("            * %d: %q", entry.LineNo, entry.Pattern))
	}
	if more := len(rules) - len(listed); more > 0 {
		points = append(points, fmt.Sprintf("            * and %d more", more))
	}

	return strings.Join(points, "\n")
}

// Name returns human readable name of the validator
func (AvoidShadowing) Name() string {
	return "Avoid Shadowing Checker"
}
