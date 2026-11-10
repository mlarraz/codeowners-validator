package check

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/dustin/go-humanize"

	"github.com/mlarraz/codeowners-validator/internal/ctxutil"
)

const (
	// maxCodeownersSize is the size limit of the CODEOWNERS file. GitHub doesn't load a bigger
	// file, so it doesn't request reviews from code owners, and its CODEOWNERS errors API doesn't
	// report any errors. GitHub documents the limit as 3 MB; a file of exactly 3 MiB is still loaded.
	maxCodeownersSize = 3 << 20
	// codeownersSizeWarningPercent is the percentage of the limit from which the file is reported
	// as close to the limit.
	codeownersSizeWarningPercent = 80
)

var (
	// A valid username/organization name has up to 39 characters (per GitHub Join page)
	// and is matched by the following regex: /^[a-z\d](?:[a-z\d]|-(?=[a-z\d])){0,38}$/i
	// A valid team name consists of alphanumerics, underscores and dashes
	usernameOrTeamRegexp = regexp.MustCompile(`^@(?i:[a-z\d](?:[a-z\d_-]){0,37}[a-z\d](/[a-z\d](?:[a-z\d_-]*)[a-z\d])?)$`)

	// Per: https://davidcel.is/posts/stop-validating-email-addresses-with-regex/
	// just check if there is '@' and a '.' afterwards
	emailRegexp = regexp.MustCompile(`.+@.+\..+`)
)

// ValidSyntax provides a syntax validation for CODEOWNERS file.
//
// If any line in your CODEOWNERS file contains invalid syntax, the file will not be detected and will
// not be used to request reviews. Invalid syntax includes inline comments and user or team names that do not exist on GitHub.
// GitHub doesn't use a file over the size limit either.
type ValidSyntax struct{}

// NewValidSyntax returns new ValidSyntax instance.
func NewValidSyntax() *ValidSyntax {
	return &ValidSyntax{}
}

// Check for syntax issues in your CODEOWNERS file.
func (v *ValidSyntax) Check(ctx context.Context, in Input) (Output, error) {
	var bldr OutputBuilder

	switch size := in.CodeownersSize; {
	case size > maxCodeownersSize:
		msg := fmt.Sprintf("The CODEOWNERS file is %s bytes, over GitHub's limit of %s bytes (3 MiB). GitHub ignores the file and doesn't request reviews from code owners.",
			humanize.Comma(size), humanize.Comma(maxCodeownersSize))
		bldr.ReportIssue(msg, WithKey("CODEOWNERS file over the size limit"))
	case size*100 >= maxCodeownersSize*codeownersSizeWarningPercent:
		msg := fmt.Sprintf("The CODEOWNERS file is %s bytes, %d%% of GitHub's limit of %s bytes (3 MiB). GitHub ignores a file over the limit.",
			humanize.Comma(size), size*100/maxCodeownersSize, humanize.Comma(maxCodeownersSize))
		bldr.ReportIssue(msg, WithSeverity(Warning), WithKey("CODEOWNERS file near the size limit"))
	}

	for _, entry := range in.CodeownersEntries {
		if ctxutil.ShouldExit(ctx) {
			return Output{}, ctx.Err()
		}

		if entry.Pattern == "" {
			bldr.ReportIssue("Missing pattern", WithEntry(entry), WithKey(entryKey("Missing pattern", entry)))
		}

	ownersLoop:
		for _, item := range entry.Owners {
			switch {
			case strings.EqualFold(item, "#"):
				break ownersLoop // no need to check for the rest items in this line, as they are ignored
			case strings.HasPrefix(item, "@"):
				if !usernameOrTeamRegexp.MatchString(item) {
					msg := fmt.Sprintf("Owner '%s' does not look like a GitHub username or team name", item)
					bldr.ReportIssue(msg, WithEntry(entry), WithSeverity(Warning), WithKey(entryKey(msg, entry)))
				}
			default:
				if !emailRegexp.MatchString(item) {
					msg := fmt.Sprintf("Owner '%s' does not look like an email", item)
					bldr.ReportIssue(msg, WithEntry(entry), WithKey(entryKey(msg, entry)))
				}
			}
		}
	}

	return bldr.Output(), nil
}

func (ValidSyntax) Name() string {
	return "Valid Syntax Checker"
}
