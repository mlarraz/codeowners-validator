package check

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/mlarraz/codeowners-validator/internal/ctxutil"
	"github.com/mlarraz/codeowners-validator/pkg/codeowners"
)

// DuplicatedPattern validates if CODEOWNERS file does not contain
// the duplicated lines with the same file pattern.
type DuplicatedPattern struct{}

// NewDuplicatedPattern returns instance of the DuplicatedPattern
func NewDuplicatedPattern() *DuplicatedPattern {
	return &DuplicatedPattern{}
}

// Check searches for doubles paths(patterns) in CODEOWNERS file. Patterns that GitHub matches the
// same way, such as "a/b" and "/a/b", are duplicates as well. The issue is reported on the last
// line that defines the pattern.
func (d *DuplicatedPattern) Check(ctx context.Context, in Input) (Output, error) {
	var bldr OutputBuilder

	patterns := map[string][]codeowners.Entry{}
	for _, entry := range in.CodeownersEntries {
		if ctxutil.ShouldExit(ctx) {
			return Output{}, ctx.Err()
		}

		key := canonicalPattern(entry.Pattern)
		patterns[key] = append(patterns[key], entry)
	}

	var duplicated []string
	for key, entries := range patterns {
		if len(entries) > 1 {
			duplicated = append(duplicated, key)
		}
	}
	last := func(key string) codeowners.Entry {
		entries := patterns[key]
		return entries[len(entries)-1]
	}
	sort.Slice(duplicated, func(i, j int) bool { return last(duplicated[i]).LineNo < last(duplicated[j]).LineNo })

	for _, key := range duplicated {
		entries := patterns[key]
		msg := fmt.Sprintf("Pattern %q is defined %d times in lines:\n%s", key, len(entries), d.listFormatFunc(entries))
		bldr.ReportIssue(msg, WithEntry(last(key)))
	}

	return bldr.Output(), nil
}

// canonicalPattern returns the form of the pattern that has a leading slash if the pattern is
// anchored at the root. A slash at the beginning or in the middle of a pattern anchors it, so
// GitHub matches "a/b" like "/a/b". A pattern without a slash, or with only a trailing one,
// matches at any depth.
func canonicalPattern(pattern string) string {
	if !strings.HasPrefix(pattern, "/") && strings.Contains(strings.TrimSuffix(pattern, "/"), "/") {
		return "/" + pattern
	}
	return pattern
}

// listFormatFunc is a basic formatter that outputs a bullet point list of the pattern. The
// patterns are listed as well if they're written differently.
func (d *DuplicatedPattern) listFormatFunc(es []codeowners.Entry) string {
	sameText := true
	for _, e := range es {
		sameText = sameText && e.Pattern == es[0].Pattern
	}

	points := make([]string, len(es))
	for i, e := range es {
		if sameText {
			points[i] = fmt.Sprintf("            * %d: with owners: %s", e.LineNo, e.Owners)
		} else {
			points[i] = fmt.Sprintf("            * %d: %q with owners: %s", e.LineNo, e.Pattern, e.Owners)
		}
	}

	return strings.Join(points, "\n")
}

// Name returns human readable name of the validator.
func (DuplicatedPattern) Name() string {
	return "Duplicated Pattern Checker"
}
