package check

import (
	"context"
	"fmt"

	"github.com/mlarraz/codeowners-validator/internal/ctxutil"

	"github.com/pkg/errors"
)

type FileExist struct{}

func NewFileExist() *FileExist {
	return &FileExist{}
}

func (f *FileExist) Check(ctx context.Context, in Input) (Output, error) {
	if ctxutil.ShouldExit(ctx) {
		return Output{}, ctx.Err()
	}

	files, err := in.Repo.Files(ctx)
	if err != nil {
		return Output{}, errors.Wrap(err, "while listing the files of the repository")
	}

	var bldr OutputBuilder

	matcher := newPathMatcher(files, in.CodeownersEntries)
	for i := range matcher.rules {
		p := &matcher.rules[i]
		if ctxutil.ShouldExit(ctx) {
			return Output{}, ctx.Err()
		}

		switch {
		case p.err != nil:
			msg := fmt.Sprintf("%q is not a valid CODEOWNERS pattern: %s", p.entry.Pattern, p.err)
			bldr.ReportIssue(msg, WithEntry(p.entry), WithKey(p.entry.Pattern))
		default:
			matched, err := matcher.matchesAny(p)
			if err != nil {
				return Output{}, errors.Wrapf(err, "while matching pattern %s", p.entry.Pattern)
			}
			if !matched {
				msg := fmt.Sprintf("%q does not match any files in repository", p.entry.Pattern)
				bldr.ReportIssue(msg, WithEntry(p.entry), WithKey(p.entry.Pattern))
			}
		}
	}

	return bldr.Output(), nil
}

func (*FileExist) Name() string {
	return "File Exist Checker"
}
