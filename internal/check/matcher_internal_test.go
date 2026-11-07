package check

import (
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/mlarraz/codeowners-validator/pkg/codeowners"
)

// TestLiteralTextIsNecessary checks that every path that the library matches with a pattern starts
// with the pattern's literal prefix and contains the rest of its literal text, so that skipping the
// other paths never misses a match.
func TestLiteralTextIsNecessary(t *testing.T) {
	rnd := rand.New(rand.NewPCG(1, 2)) //nolint:gosec // G404: a fixed seed makes the test data reproducible.
	patternTokens := []string{"a", "b", "ab", ".", "/", "/", "*", "?", "**", "\\*", "\\a"}
	pathTokens := []string{"a", "b", "ab", "ba", ".", "*", "/"}

	randomString := func(tokens []string, n int) string {
		var b strings.Builder
		for range n {
			b.WriteString(tokens[rnd.IntN(len(tokens))])
		}
		return b.String()
	}

	var paths []string
	for len(paths) < 300 {
		path := randomString(pathTokens, 1+rnd.IntN(8))
		if strings.HasPrefix(path, "/") || strings.HasSuffix(path, "/") || strings.Contains(path, "//") {
			continue
		}
		paths = append(paths, path)
	}

	checked := 0
	for range 3000 {
		pattern := randomString(patternTokens, 1+rnd.IntN(6))
		rule, err := parsePathRule(pattern)
		if err != nil || pattern == "/" {
			continue
		}
		p := (&pathMatcher{}).compile(codeowners.Entry{Pattern: pattern})
		if p.literal {
			continue
		}
		prefix, runs := literalText(pattern)

		for _, path := range paths {
			match, err := rule.Match(path)
			if err != nil || !match {
				continue
			}
			checked++
			if !strings.HasPrefix(path, prefix) || !containsInOrder(path[len(prefix):], runs) {
				t.Fatalf("pattern %q matches %q, which doesn't have its literal text: prefix %q, runs %q", pattern, path, prefix, runs)
			}
		}
	}

	if checked < 1000 {
		t.Fatalf("only %d matches were checked", checked)
	}
}
