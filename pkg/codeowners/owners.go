package codeowners

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	"github.com/dustin/go-humanize/english"
	"github.com/spf13/afero"
)

// Used for testing purposes
var fs = afero.NewOsFs()

// Entry contains owners for a given pattern
type Entry struct {
	LineNo  uint64
	Pattern string
	Owners  []string
}

func (e Entry) String() string {
	return fmt.Sprintf("line %d: %s\t%v", e.LineNo, e.Pattern, strings.Join(e.Owners, ", "))
}

// NewFromPath returns entries from codeowners
func NewFromPath(repoPath string) ([]Entry, error) {
	file, err := FindFile(repoPath)
	if err != nil {
		return nil, err
	}

	f, err := fs.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	return ParseCodeowners(f), nil
}

// FindFile returns the path of the CODEOWNERS file in the root, docs/, or .github/ directory of
// the repository.
// see: https://help.github.com/articles/about-code-owners/#codeowners-file-location
func FindFile(repoPath string) (string, error) {
	var detectedFiles []string
	for _, p := range []string{".", "docs", ".github"} {
		pth := path.Join(repoPath, p)
		exists, err := afero.DirExists(fs, pth)
		if err != nil {
			return "", err
		}

		if !exists {
			continue
		}

		f := path.Join(pth, "CODEOWNERS")
		_, err = fs.Stat(f)
		switch {
		case err == nil:
		case os.IsNotExist(err):
			continue
		default:
			return "", err
		}

		detectedFiles = append(detectedFiles, f)
	}

	switch l := len(detectedFiles); l {
	case 0:
		return "", fmt.Errorf("no CODEOWNERS found in the root, docs/, or .github/ directory of the repository %s", repoPath)
	case 1:
		return detectedFiles[0], nil
	default:
		return "", fmt.Errorf("multiple CODEOWNERS files found in the %s locations of the repository %s",
			english.OxfordWordSeries(replacePrefix(detectedFiles, repoPath, "./"), "and"),
			repoPath)
	}
}

func replacePrefix(in []string, prefix, s string) []string {
	for idx := range in {
		in[idx] = fmt.Sprintf("%s%s", s, strings.TrimPrefix(in[idx], prefix))
	}
	return in
}

func ParseCodeowners(r io.Reader) []Entry {
	var e []Entry
	s := bufio.NewScanner(r)
	no := uint64(0)
	for s.Scan() {
		no++
		fields := strings.Fields(s.Text())

		if len(fields) == 0 { // empty
			continue
		}

		if strings.HasPrefix(fields[0], "#") { // comment
			continue
		}

		n := len(fields)
		for idx, x := range fields {
			if !strings.HasPrefix(x, "#") {
				continue
			}
			n = idx
		}

		e = append(e, Entry{
			Pattern: fields[0],
			Owners:  fields[1:n],
			LineNo:  no,
		})
	}

	return e
}
