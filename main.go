package main

import (
	"context"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"go.szostok.io/version/extension"

	"github.com/mlarraz/codeowners-validator/internal/check"
	"github.com/mlarraz/codeowners-validator/internal/envconfig"
	"github.com/mlarraz/codeowners-validator/internal/load"
	"github.com/mlarraz/codeowners-validator/internal/runner"
	"github.com/mlarraz/codeowners-validator/pkg/codeowners"
)

// Config holds the application configuration
type Config struct {
	RepositoryPath     string
	CheckFailureLevel  check.SeverityType `envconfig:"default=warning"`
	Checks             []string           `envconfig:"optional"`
	ExperimentalChecks []string           `envconfig:"optional"`
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
			var cfg Config
			err := envconfig.Init(&cfg)
			exitOnError(err)

			log := logrus.New()

			// init checks
			checks, err := load.Checks(cmd.Context(), cfg.Checks, cfg.ExperimentalChecks)
			exitOnError(err)

			// init codeowners entries
			codeownersEntries, err := codeowners.NewFromPath(cfg.RepositoryPath)
			exitOnError(err)

			// run check runner
			absRepoPath, err := filepath.Abs(cfg.RepositoryPath)
			exitOnError(err)

			checkRunner := runner.NewCheckRunner(log, codeownersEntries, absRepoPath, cfg.CheckFailureLevel, checks...)
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
	)

	return rootCmd
}
