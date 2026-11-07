package check

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/mlarraz/codeowners-validator/internal/ctxutil"
	"github.com/mlarraz/codeowners-validator/pkg/codeowners"

	hmarr "github.com/hmarr/codeowners"
)

// anyOwner accepts every owner token. It's used to compile a CODEOWNERS pattern on its own; the
// owners don't matter for matching files.
var anyOwner = hmarr.OwnerMatchFunc(func(s string) (hmarr.Owner, error) {
	return hmarr.Owner{Value: s, Type: hmarr.UsernameOwner}, nil
})

// fileRange is a half-open range of indexes into pathMatcher.files.
type fileRange struct {
	lo, hi int
}

// pathPattern is a CODEOWNERS entry compiled for matching against repository files.
type pathPattern struct {
	entry codeowners.Entry
	// match reports whether the pattern matches the given slash-separated path. It's nil when the
	// pattern can't be parsed.
	match func(string) (bool, error)
	// ranges are the ranges of files that the pattern can match. For literal patterns, they are
	// exactly the files that the pattern matches; otherwise they're a superset that has to be
	// tested with match.
	ranges []fileRange
	// literal reports whether every file in ranges is matched by the pattern.
	literal bool
	// err explains why the pattern can't be matched, if it can't.
	err error
}

// pathMatcher matches CODEOWNERS patterns against the files tracked in the repository.
//
// GitHub matches CODEOWNERS patterns with the rules of gitignore; the patterns are compiled and
// matched by github.com/hmarr/codeowners. Comparing every pattern with every file doesn't scale to
// repositories with hundreds of thousands of files, so the sorted file list is indexed instead:
// left-anchored literal patterns are resolved with binary search, and patterns with a wildcard are
// only tested against the files below their literal leading directories.
type pathMatcher struct {
	files []string
	rules []pathPattern
}

// newPathMatcher compiles the CODEOWNERS entries for matching against files, which must be sorted
// bytewise, as returned by repo.Repo.Files.
func newPathMatcher(files []string, entries []codeowners.Entry) *pathMatcher {
	m := &pathMatcher{files: files}
	m.rules = make([]pathPattern, len(entries))
	for i, entry := range entries {
		m.rules[i] = m.compile(entry)
	}
	return m
}

// parsePathRule compiles a CODEOWNERS pattern with the library that implements GitHub's matching
// semantics. The library only parses whole CODEOWNERS lines, so the pattern is parsed on its own
// with an owner matcher that accepts anything.
func parsePathRule(pattern string) (hmarr.Rule, error) {
	rules, err := hmarr.ParseFile(
		strings.NewReader(pattern+" x"),
		hmarr.WithOwnerMatchers([]hmarr.OwnerMatcher{anyOwner}),
	)
	if err != nil {
		// ParseFile wraps parse errors with the number of the line it parsed, which is always the
		// one line built here.
		if inner := errors.Unwrap(err); inner != nil {
			return hmarr.Rule{}, inner
		}
		return hmarr.Rule{}, err
	}

	if len(rules) != 1 || rules[0].RawPattern() != pattern {
		return hmarr.Rule{}, fmt.Errorf("the pattern is not parsed as a single path pattern")
	}
	return rules[0], nil
}

func (m *pathMatcher) compile(entry codeowners.Entry) pathPattern {
	p := pathPattern{entry: entry}

	rule, err := parsePathRule(entry.Pattern)
	if err != nil {
		p.err = err
		return p
	}

	if entry.Pattern == "/" {
		// "/" is a valid pattern that doesn't match anything, but the library's matcher panics on
		// it.
		p.literal = true
		return p
	}
	p.match = rule.Match

	if !strings.ContainsAny(entry.Pattern, "*?\\") && strings.HasPrefix(entry.Pattern, "/") {
		// The library treats this pattern as a left-anchored literal: it matches the path itself
		// and, unless the pattern has a trailing slash, the paths below it.
		p.literal = true
		path := strings.TrimPrefix(entry.Pattern, "/")
		if strings.HasSuffix(path, "/") {
			p.ranges = m.prefixRanges(path)
		} else {
			p.ranges = m.exactRange(path)
			p.ranges = append(p.ranges, m.prefixRanges(path+"/")...)
		}
		return p
	}

	p.ranges = m.candidateRanges(entry.Pattern)
	return p
}

// candidateRanges returns the ranges of files that may be matched by the given valid pattern.
func (m *pathMatcher) candidateRanges(pattern string) []fileRange {
	// Patterns without a leading slash are anchored at the root as well, unless they consist of a
	// single segment (optionally with a trailing slash), which matches at any depth.
	path := strings.TrimPrefix(pattern, "/")
	if !strings.HasPrefix(pattern, "/") && !strings.Contains(strings.TrimSuffix(path, "/"), "/") {
		return []fileRange{{0, len(m.files)}}
	}

	// Every path matched by a root-anchored pattern starts with the literal leading directories
	// of the pattern, the ones that come before the first segment with a wildcard.
	segments := strings.Split(path, "/")
	segments = segments[:len(segments)-1]
	var prefix strings.Builder
	for _, segment := range segments {
		if segment == "" || strings.ContainsAny(segment, "*?\\") {
			break
		}
		prefix.WriteString(segment)
		prefix.WriteByte('/')
	}
	if prefix.Len() == 0 {
		return []fileRange{{0, len(m.files)}}
	}
	return m.prefixRanges(prefix.String())
}

// prefixRanges returns the range of files that start with the given prefix.
func (m *pathMatcher) prefixRanges(prefix string) []fileRange {
	lo := sort.SearchStrings(m.files, prefix)
	if lo == len(m.files) || !strings.HasPrefix(m.files[lo], prefix) {
		return nil
	}
	// The path separator is 0x2f, so prefix+"\xff" sorts after every file that starts with the
	// prefix, and before every file that sorts after them without starting with it.
	hi := sort.SearchStrings(m.files, prefix+"\xff")
	return []fileRange{{lo, hi}}
}

// exactRange returns the range of the file that has the given path, if it's tracked.
func (m *pathMatcher) exactRange(path string) []fileRange {
	i := sort.SearchStrings(m.files, path)
	if i == len(m.files) || m.files[i] != path {
		return nil
	}
	return []fileRange{{i, i + 1}}
}

// matchesAny reports whether at least one file is matched by the pattern.
func (m *pathMatcher) matchesAny(p *pathPattern) (bool, error) {
	if p.literal {
		return len(p.ranges) > 0, nil
	}

	for _, r := range p.ranges {
		for _, file := range m.files[r.lo:r.hi] {
			match, err := p.match(file)
			if err != nil {
				return false, err
			}
			if match {
				return true, nil
			}
		}
	}
	return false, nil
}

// notOwned returns the sorted paths of the files that aren't matched by any pattern accepted by
// keep. Only the files accepted by include are considered.
func (m *pathMatcher) notOwned(ctx context.Context, keep func(*pathPattern) bool, include func(string) bool) ([]string, error) {
	owned := make([]bool, len(m.files))
	unowned := 0
	for i, file := range m.files {
		owned[i] = !include(file)
		if !owned[i] {
			unowned++
		}
	}

	// Literal patterns match every file in their ranges, so mark those files first and don't test
	// them again.
	for i := range m.rules {
		p := &m.rules[i]
		if !p.literal || !keep(p) {
			continue
		}
		for _, r := range p.ranges {
			for j := r.lo; j < r.hi; j++ {
				if !owned[j] {
					owned[j] = true
					unowned--
				}
			}
		}
	}

	// Test only the files that are still unowned against the patterns with a wildcard.
	for i := range m.rules {
		p := &m.rules[i]
		if ctxutil.ShouldExit(ctx) {
			return nil, ctx.Err()
		}
		if p.literal || p.err != nil || !keep(p) || unowned == 0 {
			continue
		}
		for _, r := range p.ranges {
			for j := r.lo; j < r.hi; j++ {
				if owned[j] {
					continue
				}
				match, err := p.match(m.files[j])
				if err != nil {
					return nil, err
				}
				if match {
					owned[j] = true
					unowned--
				}
			}
			if unowned == 0 {
				break
			}
		}
	}

	unownedFiles := make([]string, 0, unowned)
	for i, file := range m.files {
		if !owned[i] {
			unownedFiles = append(unownedFiles, file)
		}
	}
	return unownedFiles, nil
}
