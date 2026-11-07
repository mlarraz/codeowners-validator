package load

import (
	"context"

	"github.com/mlarraz/codeowners-validator/internal/check"
	"github.com/mlarraz/codeowners-validator/internal/envconfig"
	"github.com/mlarraz/codeowners-validator/internal/github"

	"github.com/pkg/errors"
)

// defaultChecks are the checks that run when none are listed.
var defaultChecks = []string{"syntax", "duppatterns", "files", "owners"}

// Checks returns the listed checks, or the default checks if none are listed. Only the listed
// checks read their configuration and create clients.
func Checks(ctx context.Context, enabledChecks []string) ([]check.Checker, error) {
	if len(enabledChecks) == 0 {
		enabledChecks = defaultChecks
	}

	var checks []check.Checker

	if contains(enabledChecks, "syntax") {
		checks = append(checks, check.NewValidSyntax())
	}

	if contains(enabledChecks, "duppatterns") {
		checks = append(checks, check.NewDuplicatedPattern())
	}

	if contains(enabledChecks, "files") {
		checks = append(checks, check.NewFileExist())
	}

	if contains(enabledChecks, "owners") {
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

		checks = append(checks, owners)
	}

	if contains(enabledChecks, "notowned") {
		var cfg struct {
			NotOwnedChecker check.NotOwnedFileConfig
		}
		if err := envconfig.Init(&cfg); err != nil {
			return nil, errors.Wrapf(err, "while loading config for %s", "notowned")
		}

		checks = append(checks, check.NewNotOwnedFile(cfg.NotOwnedChecker))
	}

	if contains(enabledChecks, "avoid-shadowing") {
		checks = append(checks, check.NewAvoidShadowing())
	}

	return checks, nil
}

func contains(checks []string, name string) bool {
	for _, c := range checks {
		if c == name {
			return true
		}
	}
	return false
}
