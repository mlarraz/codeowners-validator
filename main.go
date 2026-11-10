package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"syscall"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"go.szostok.io/version/extension"

	"github.com/mlarraz/codeowners-validator/internal/baseline"
	"github.com/mlarraz/codeowners-validator/internal/check"
	"github.com/mlarraz/codeowners-validator/internal/envconfig"
	"github.com/mlarraz/codeowners-validator/internal/load"
	"github.com/mlarraz/codeowners-validator/internal/printer"
	"github.com/mlarraz/codeowners-validator/internal/repo"
	"github.com/mlarraz/codeowners-validator/internal/runner"
	"github.com/mlarraz/codeowners-validator/pkg/codeowners"
)

// Config holds the application configuration
type Config struct {
	RepositoryPath    string
	CheckFailureLevel check.SeverityType `envconfig:"default=warning"`
	Checks            []string           `envconfig:"optional"`
	// ExperimentalChecks was removed when its checks became regular checks. It's still read so that
	// setting it fails instead of silently not running the checks.
	ExperimentalChecks []string `envconfig:"optional"`
	// TrustWorkspace marks the repository as a safe directory for Git, which is needed when it's
	// owned by another user, for example when it's mounted into a container.
	TrustWorkspace bool `envconfig:"default=false"`
	// BaselinePath is the path of the baseline file, relative to the repository path. The issues
	// that it lists don't fail the validation.
	BaselinePath string `envconfig:"optional"`
}

// baselinePath returns the path of the baseline file, or "" if there's none.
func (c *Config) baselinePath() string {
	if c.BaselinePath == "" || filepath.IsAbs(c.BaselinePath) {
		return c.BaselinePath
	}
	return filepath.Join(c.RepositoryPath, c.BaselinePath)
}

func main() {
	ctx, cancelFunc := WithStopContext(context.Background())
	defer cancelFunc()

	if err := NewRoot().ExecuteContext(ctx); err != nil {
		// error is already handled by `cobra`, we don't want to log it here as we will duplicate the message.
		// If needed, based on error type we can exit with different codes.
		//nolint:gocritic
		os.Exit(1)
	}
}

func exitOnError(err error) {
	if err != nil {
		logrus.Fatal(err)
	}
}

// WithStopContext returns a copy of parent with a new Done channel. The returned
// context's Done channel is closed on of SIGINT or SIGTERM signals.
func WithStopContext(parent context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		select {
		case <-ctx.Done():
		case <-sigCh:
			cancel()
		}
	}()

	return ctx, cancel
}

// NewRoot returns a root cobra.Command for the whole Agent utility.
func NewRoot() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:          "codeowners-validator",
		Short:        "Ensures the correctness of your CODEOWNERS file.",
		SilenceUsage: true,
		Run: func(cmd *cobra.Command, _ []string) {
			cfg := loadConfig()

			var known baseline.Baseline
			if path := cfg.baselinePath(); path != "" {
				var err error
				known, err = baseline.Load(path)
				exitOnError(err)
			}

			log := logrus.New()

			checks, in, codeownersPath := setup(cmd.Context(), cfg, known)
			p, err := newPrinter(codeownersPath)
			exitOnError(err)

			checkers := make([]check.Checker, len(checks))
			for i, c := range checks {
				checkers[i] = c.Checker
			}
			checkRunner := runner.NewCheckRunner(log, in, cfg.CheckFailureLevel, p, checkers...)
			checkRunner.Run(cmd.Context())

			if cmd.Context().Err() != nil {
				log.Error("Application was interrupted by operating system")
				os.Exit(2)
			}
			if checkRunner.ShouldExitWithCheckFailure() {
				os.Exit(3)
			}
		},
	}

	rootCmd.AddCommand(
		extension.NewVersionCobraCmd(),
		newBaselineCmd(),
	)

	return rootCmd
}

// newBaselineCmd returns the command that writes the issues of the listed checks to the baseline.
func newBaselineCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "baseline",
		Short: "Writes the current issues of the listed checks to the baseline file at BASELINE_PATH.",
		Long: "Runs the checks listed in CHECKS, with the same configuration as the validation, and writes the keys of their issues to the baseline file at BASELINE_PATH. " +
			"The issues of checks that aren't listed are kept. Issues without a key, such as the files that notowned reports, can't be baselined.",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg := loadConfig()
			path := cfg.baselinePath()
			if path == "" {
				return errors.New("set BASELINE_PATH to the path of the baseline file")
			}

			// Keep the issues of the checks that don't run.
			known := baseline.Baseline{}
			if _, err := os.Stat(path); err == nil {
				known, err = baseline.Load(path)
				if err != nil {
					return err
				}
			}

			checks, in, _ := setup(cmd.Context(), cfg, nil)
			out := cmd.OutOrStdout()
			for _, c := range checks {
				res, err := c.Checker.Check(cmd.Context(), in)
				if err != nil {
					return fmt.Errorf("while running %s: %w", c.Name, err)
				}

				var keys []string
				unkeyed := 0
				for _, i := range res.Issues {
					if i.Key == "" {
						unkeyed++
						continue
					}
					keys = append(keys, i.Key)
				}
				delete(known, c.Name)
				if len(keys) > 0 {
					known[c.Name] = keys
				}

				fmt.Fprintf(out, "%s: %d issue(s) in the baseline", c.Name, len(keys))
				if unkeyed > 0 {
					fmt.Fprintf(out, ", %d issue(s) that can't be baselined", unkeyed)
				}
				fmt.Fprintln(out)
			}

			if err := baseline.Write(path, known); err != nil {
				return err
			}

			names := make([]string, 0, len(known))
			for name := range known {
				names = append(names, name)
			}
			sort.Strings(names)
			fmt.Fprintf(out, "Wrote %s with the issues of: %v\n", path, names)
			return nil
		},
	}
}

// loadConfig reads the configuration from the environment.
func loadConfig() *Config {
	cfg := &Config{}
	exitOnError(envconfig.Init(cfg))
	if len(cfg.ExperimentalChecks) > 0 {
		exitOnError(errors.New("EXPERIMENTAL_CHECKS (the experimental_checks input of the GitHub Action) was removed: notowned and avoid-shadowing are regular checks now, list them in CHECKS (checks)"))
	}
	return cfg
}

// setup returns the listed checks, their input, and the path of the CODEOWNERS file.
func setup(ctx context.Context, cfg *Config, known baseline.Baseline) ([]load.Check, check.Input, string) {
	checks, err := load.Checks(ctx, cfg.Checks, known)
	exitOnError(err)

	codeownersPath, err := codeowners.FindFile(cfg.RepositoryPath)
	exitOnError(err)
	content, err := os.ReadFile(codeownersPath)
	exitOnError(err)

	absRepoPath, err := filepath.Abs(cfg.RepositoryPath)
	exitOnError(err)

	in := check.Input{
		Repo:              repo.New(absRepoPath, cfg.TrustWorkspace),
		CodeownersEntries: codeowners.ParseCodeowners(bytes.NewReader(content)),
		CodeownersSize:    int64(len(content)),
	}
	return checks, in, codeownersPath
}

// newPrinter returns the printer for the check results. In GitHub Actions, the results are also
// reported as annotations on the CODEOWNERS file and added to the job summary.
func newPrinter(codeownersPath string) (runner.Printer, error) {
	if os.Getenv("GITHUB_ACTIONS") != "true" {
		return &printer.TTYPrinter{}, nil
	}

	file, err := filepath.Abs(codeownersPath)
	if err != nil {
		return nil, err
	}
	return printer.NewGitHubActionsPrinter(file, os.Getenv("GITHUB_STEP_SUMMARY")), nil
}
