package load

import (
	"context"
	"slices"
	"strings"

	"github.com/mlarraz/codeowners-validator/internal/baseline"
	"github.com/mlarraz/codeowners-validator/internal/check"
	"github.com/mlarraz/codeowners-validator/internal/envconfig"
	"github.com/mlarraz/codeowners-validator/internal/github"

	"github.com/pkg/errors"
)

// defaultChecks are the checks that run when none are listed.
var defaultChecks = []string{"syntax", "duppatterns", "files", "owners"}

// allChecks are the names of all checks.
var allChecks = []string{"syntax", "duppatterns", "files", "owners", "notowned", "avoid-shadowing"}

// Check is a check and the name that it's listed with in CHECKS and in the baseline.
type Check struct {
	Name    string
	Checker check.Checker
}

// Checks returns the listed checks, or the default checks if none are listed. Only the listed
// checks read their configuration and create clients.
//
// A check can be listed with a level, as name:level, to report its issues at that level at most,
// see check.ParseLevel. The issues listed in known are reported as baselined, see
// check.WithBaseline.
func Checks(ctx context.Context, enabledChecks []string, known baseline.Baseline) ([]Check, error) {
	if len(enabledChecks) == 0 {
		enabledChecks = defaultChecks
	}

	for name := range known {
		if !slices.Contains(allChecks, name) {
			return nil, errors.Errorf("the baseline lists issues of %q, which isn't a check; possible checks: %s", name, strings.Join(allChecks, ","))
		}
	}

	levels := make(map[string]check.SeverityType, len(enabledChecks))
	for _, c := range enabledChecks {
		name, level, hasLevel := strings.Cut(c, ":")
		levels[name] = 0
		if !hasLevel {
			continue
		}
		l, err := check.ParseLevel(level)
		if err != nil {
			return nil, errors.Wrapf(err, "while reading the level of the %s check", name)
		}
		levels[name] = l
	}

	enabled := func(name string) bool {
		_, ok := levels[name]
		return ok
	}

	var checks []Check
	add := func(name string, c check.Checker) {
		if keys, ok := known[name]; ok {
			c = check.WithBaseline(c, keys)
		}
		// The level applies to the baseline's warnings about stale entries too.
		if level := levels[name]; level != 0 {
			c = check.WithLevel(c, level)
		}
		checks = append(checks, Check{Name: name, Checker: c})
	}

	if enabled("syntax") {
		add("syntax", check.NewValidSyntax())
	}

	if enabled("duppatterns") {
		add("duppatterns", check.NewDuplicatedPattern())
	}

	if enabled("files") {
		add("files", check.NewFileExist())
	}

	if enabled("owners") {
		var cfg struct {
			OwnerChecker check.ValidOwnerConfig
			Github       github.ClientConfig
		}
		if err := envconfig.Init(&cfg); err != nil {
			return nil, errors.Wrapf(err, "while loading config for %s", "owners")
		}

		ghClient, err := github.NewClient(ctx, &cfg.Github)
		if err != nil {
			return nil, errors.Wrap(err, "while creating GitHub client")
		}

		owners, err := check.NewValidOwner(cfg.OwnerChecker, ghClient)
		if err != nil {
			return nil, errors.Wrap(err, "while enabling 'owners' checker")
		}

		add("owners", owners)
	}

	if enabled("notowned") {
		var cfg struct {
			NotOwnedChecker check.NotOwnedFileConfig
		}
		if err := envconfig.Init(&cfg); err != nil {
			return nil, errors.Wrapf(err, "while loading config for %s", "notowned")
		}

		add("notowned", check.NewNotOwnedFile(cfg.NotOwnedChecker))
	}

	if enabled("avoid-shadowing") {
		add("avoid-shadowing", check.NewAvoidShadowing())
	}

	return checks, nil
}
