package check

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/mlarraz/codeowners-validator/internal/ctxutil"

	"github.com/pkg/errors"
)

type NotOwnedFileConfig struct {
	SkipPatterns   []string `envconfig:"optional"`
	Subdirectories []string `envconfig:"optional"`
}

type NotOwnedFile struct {
	skipPatterns   map[string]struct{}
	subDirectories []string
}

func NewNotOwnedFile(cfg NotOwnedFileConfig) *NotOwnedFile {
	skip := map[string]struct{}{}
	for _, p := range cfg.SkipPatterns {
		skip[p] = struct{}{}
	}

	return &NotOwnedFile{
		skipPatterns:   skip,
		subDirectories: cfg.Subdirectories,
	}
}

func (c *NotOwnedFile) Check(ctx context.Context, in Input) (Output, error) {
	if ctxutil.ShouldExit(ctx) {
		return Output{}, ctx.Err()
	}

	var bldr OutputBuilder

	if len(in.CodeownersEntries) == 0 {
		bldr.ReportIssue("The CODEOWNERS file is empty. The files in the repository don't have any owner.")
		return bldr.Output(), nil
	}

	files, err := in.Repo.Files(ctx)
	if err != nil {
		return Output{}, errors.Wrap(err, "while listing the files of the repository")
	}

	matcher := newPathMatcher(files, in.CodeownersEntries)
	notOwned, err := matcher.notOwned(ctx, c.isConsidered, c.isInSubdirectories)
	if err != nil {
		return Output{}, errors.Wrap(err, "while looking for files that don't have any owner")
	}

	if len(notOwned) > 0 {
		msg := fmt.Sprintf("Found %d not owned files (skipped patterns: %q):\n%s", len(notOwned), c.skipPatternsList(), c.ListFormatFunc(notOwned))
		bldr.ReportIssue(msg)
	}

	return bldr.Output(), nil
}

// isConsidered reports whether the entry takes part in the check. Skipped patterns are ignored,
// which turns the files that they match into not owned files.
func (c *NotOwnedFile) isConsidered(p *pathPattern) bool {
	_, skipped := c.skipPatterns[p.entry.Pattern]
	return !skipped
}

// isInSubdirectories reports whether the file is under one of the configured subdirectories.
func (c *NotOwnedFile) isInSubdirectories(file string) bool {
	if len(c.subDirectories) == 0 {
		return true
	}

	for _, dir := range c.subDirectories {
		dir = strings.TrimSuffix(strings.TrimPrefix(dir, "./"), "/")
		if dir == "" || dir == "." || file == dir || strings.HasPrefix(file, dir+"/") {
			return true
		}
	}
	return false
}

func (c *NotOwnedFile) skipPatternsList() string {
	list := make([]string, 0, len(c.skipPatterns))
	for k := range c.skipPatterns {
		list = append(list, k)
	}
	sort.Strings(list)
	return strings.Join(list, ",")
}

// ListFormatFunc is a basic formatter that outputs
// a bullet point list of the pattern.
func (c *NotOwnedFile) ListFormatFunc(es []string) string {
	points := make([]string, len(es))
	for i, err := range es {
		points[i] = fmt.Sprintf("            * %s", err)
	}

	return strings.Join(points, "\n")
}

// Name returns human-readable name of the validator
func (NotOwnedFile) Name() string {
	return "Not Owned File Checker"
}
