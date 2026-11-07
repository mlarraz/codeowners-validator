//go:build integration
// +build integration

package integration

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/sebdah/goldie/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	binaryPathEnvName = "BINARY_PATH"

	// Sample repositories under testdata/repos, one directory per GitHub organization.
	codeownersSamplesRepo                   = "gh-codeowners"
	caseInsensitiveOrgCodeownersSamplesRepo = "GitHubCODEOWNERS"

	gitHubAppId                = "190766"
	gitHubAppInstallationId    = "24938181"
	gitHubAppPrivateKeyEnvName = "APP_PRIVATE_KEY"
)

var repositories = []struct {
	name string
	repo string
}{
	{
		name: "gh-codeowners",
		repo: codeownersSamplesRepo,
	},
	{
		name: "GitHubCODEOWNERS",
		repo: caseInsensitiveOrgCodeownersSamplesRepo,
	},
}

// TestCheckHappyPath tests that codeowners-validator reports no issues for valid CODEOWNERS file.
//
// This test is based on golden file.
// If the `-test.update-golden` flag is set then the actual content is written
// to the golden file.
func TestCheckSuccess(t *testing.T) {
	type (
		Envs     map[string]string
		testCase []struct {
			name   string
			repo   string
			envs   Envs
			skipOS string
		}
	)

	// To update golden file, run:
	//   TEST=TestCheckSuccess/offline_checks UPDATE_GOLDEN=true make test-integration
	t.Run("offline checks", func(t *testing.T) {
		for _, repoTC := range repositories {
			// given
			repoDir := SampleRepo(t, repoTC.repo, "happy-path")

			tests := testCase{
				{
					name: "files",
					envs: Envs{
						"CHECKS": "files",
					},
				},
				{
					name: "duppatterns",
					envs: Envs{
						"CHECKS": "duppatterns",
					},
				},
				{
					name: "avoid-shadowing",
					envs: Envs{
						"CHECKS":              "disable-all",
						"EXPERIMENTAL_CHECKS": "avoid-shadowing",
					},
				},
				{
					name: "notowned",
					envs: Envs{
						"PATH":                os.Getenv("PATH"), // need to be set to find the `git` binary
						"CHECKS":              "disable-all",
						"EXPERIMENTAL_CHECKS": "notowned",
					},
					skipOS: "windows",
				},
			}
			for _, tc := range tests {
				t.Run(fmt.Sprintf("%s/%s", repoTC.name, tc.name), func(t *testing.T) {
					if runtime.GOOS == tc.skipOS {
						t.Skip("this test is marked as skipped for this OS")
					}

					binaryPath := os.Getenv(binaryPathEnvName)
					codeownersCmd := Exec().
						Binary(binaryPath).
						// codeowners-validator basic config
						WithEnv("REPOSITORY_PATH", repoDir)

					for k, v := range tc.envs {
						codeownersCmd.WithEnv(k, v)
					}

					// when
					result := codeownersCmd.AwaitResultAtMost(3 * time.Minute)

					// then
					assert.Equal(t, 0, result.ExitCode)
					normalizedOutput := normalizeTimeDurations(result.Stdout)

					g := goldie.New(t, goldie.WithNameSuffix(".golden.txt"))
					g.Assert(t, t.Name(), []byte(normalizedOutput))
				})
			}
		}
	})

	// To update golden file, run:
	//   GITHUB_TOKEN=<token> TEST=TestCheckSuccess/online_checks UPDATE_GOLDEN=true make test-integration
	t.Run("online checks", func(t *testing.T) {
		tests := testCase{
			{
				name: "gh-codeowners/owners",
				envs: Envs{
					"CHECKS":                   "owners",
					"OWNER_CHECKER_REPOSITORY": "gh-codeowners/codeowners-samples",
					"OWNER_CHECKER_REF":        "happy-path",
					"GITHUB_ACCESS_TOKEN":      os.Getenv("GITHUB_TOKEN"),
				},
				repo: codeownersSamplesRepo,
			},
			{
				name: "GitHubCODEOWNERS/owners",
				envs: Envs{
					"CHECKS":                   "owners",
					"OWNER_CHECKER_REPOSITORY": "GitHubCODEOWNERS/codeowners-samples",
					"OWNER_CHECKER_REF":        "happy-path",
					"GITHUB_ACCESS_TOKEN":      os.Getenv("GITHUB_TOKEN"),
				},
				repo: caseInsensitiveOrgCodeownersSamplesRepo,
			},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				getEnvOrSkip(t, "GITHUB_TOKEN")

				// given
				repoDir := SampleRepo(t, tc.repo, "happy-path")

				if runtime.GOOS == tc.skipOS {
					t.Skip("this test is marked as skipped for this OS")
				}

				binaryPath := os.Getenv(binaryPathEnvName)
				codeownersCmd := Exec().
					Binary(binaryPath).
					// codeowners-validator basic config
					WithEnv("REPOSITORY_PATH", repoDir)

				for k, v := range tc.envs {
					codeownersCmd.WithEnv(k, v)
				}

				// when
				result := codeownersCmd.AwaitResultAtMost(3 * time.Minute)

				// then
				assert.Equal(t, 0, result.ExitCode)
				normalizedOutput := normalizeTimeDurations(result.Stdout)

				g := goldie.New(t, goldie.WithNameSuffix(".golden.txt"))
				g.Assert(t, t.Name(), []byte(normalizedOutput))
			})
		}
	})
}

// TestCheckFailures tests that codeowners-validator reports issues for not valid CODEOWNERS file.
//
// This test is based on golden file.
// If the `-test.update-golden` flag is set then the actual content is written
// to the golden file.
//
// To update golden file, run:
//
//	TEST=TestCheckFailures UPDATE_GOLDEN=true make test-integration
func TestCheckFailures(t *testing.T) {
	type Envs map[string]string
	tests := []struct {
		name string
		envs Envs
	}{
		{
			name: "files",
			envs: Envs{
				"CHECKS": "files",
			},
		},
		{
			name: "owners",
			envs: Envs{
				"CHECKS":                               "owners",
				"OWNER_CHECKER_REPOSITORY":             "gh-codeowners/codeowners-samples",
				"OWNER_CHECKER_REF":                    "failures",
				"OWNER_CHECKER_ALLOW_UNOWNED_PATTERNS": "false",
				"GITHUB_ACCESS_TOKEN":                  os.Getenv("GITHUB_TOKEN"),
			},
		},
		{
			name: "duppatterns",
			envs: Envs{
				"CHECKS": "duppatterns",
			},
		},
		{
			name: "avoid-shadowing",
			envs: Envs{
				"CHECKS":              "disable-all",
				"EXPERIMENTAL_CHECKS": "avoid-shadowing",
			},
		},
		{
			name: "notowned",
			envs: Envs{
				"PATH":                            os.Getenv("PATH"), // need to be set to find the `git` binary
				"CHECKS":                          "disable-all",
				"EXPERIMENTAL_CHECKS":             "notowned",
				"NOT_OWNED_CHECKER_SKIP_PATTERNS": "*",
			},
		},
		{
			name: "notowned_sub_dirs",
			envs: Envs{
				"PATH":                             os.Getenv("PATH"), // need to be set to find the `git` binary
				"CHECKS":                           "disable-all",
				"EXPERIMENTAL_CHECKS":              "notowned",
				"NOT_OWNED_CHECKER_SKIP_PATTERNS":  "*",
				"NOT_OWNED_CHECKER_SUBDIRECTORIES": "notowned/dir",
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, needsToken := tc.envs["GITHUB_ACCESS_TOKEN"]; needsToken {
				getEnvOrSkip(t, "GITHUB_TOKEN")
			}

			// given
			repoDir := SampleRepo(t, codeownersSamplesRepo, "failures")

			binaryPath := os.Getenv(binaryPathEnvName)

			codeownersCmd := Exec().
				Binary(binaryPath).
				// codeowners-validator basic config
				WithEnv("REPOSITORY_PATH", repoDir)

			for k, v := range tc.envs {
				codeownersCmd.WithEnv(k, v)
			}

			// when
			result := codeownersCmd.AwaitResultAtMost(3 * time.Minute)

			// then
			assert.Equal(t, 3, result.ExitCode)

			normalizedOutput := normalizeTimeDurations(result.Stdout)

			g := goldie.New(t, goldie.WithNameSuffix(".golden.txt"))
			g.Assert(t, t.Name(), []byte(normalizedOutput))
		})
	}
}

// TestCheckSparseBloblessClone tests that the checks that inspect the files of the repository
// don't need a full checkout: a blobless clone that contains only the CODEOWNERS file gives the
// same result as the full checkout.
func TestCheckSparseBloblessClone(t *testing.T) {
	t.Parallel()

	repoDir := SampleRepo(t, codeownersSamplesRepo, "failures")
	cloneDir := BloblessSparseClone(t, repoDir, "CODEOWNERS")
	require.NoFileExists(t, filepath.Join(cloneDir, "action.yml"))

	tests := []struct {
		name         string
		envs         map[string]string
		wantContains string
	}{
		{
			name: "files",
			envs: map[string]string{
				"PATH":   os.Getenv("PATH"), // need to be set to find the `git` binary
				"CHECKS": "files",
			},
			wantContains: `"/this-folder-does-not-exits/really" does not match any files in repository`,
		},
		{
			name: "notowned",
			envs: map[string]string{
				"PATH":                            os.Getenv("PATH"), // need to be set to find the `git` binary
				"CHECKS":                          "disable-all",
				"EXPERIMENTAL_CHECKS":             "notowned",
				"NOT_OWNED_CHECKER_SKIP_PATTERNS": "*",
			},
			wantContains: "Found 4 not owned files",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			run := func(dir string) *ExecuteOutput {
				codeownersCmd := Exec().
					Binary(os.Getenv(binaryPathEnvName)).
					WithEnv("REPOSITORY_PATH", dir)

				for k, v := range tc.envs {
					codeownersCmd.WithEnv(k, v)
				}
				return codeownersCmd.AwaitResultAtMost(time.Minute)
			}

			// when
			full := run(repoDir)
			sparse := run(cloneDir)

			// then
			assert.Contains(t, sparse.Stdout, tc.wantContains)
			assert.Equal(t, normalizeTimeDurations(full.Stdout), normalizeTimeDurations(sparse.Stdout))
		})
	}
}

// TestOwnerCheckAuthZAndAuthN verifies that the owners check works against public repositories
// without credentials and reports rejected ones.
//
// To update golden file, run:
//
//	TEST=TestOwnerCheckAuthZAndAuthN UPDATE_GOLDEN=true make test-integration
func TestOwnerCheckAuthZAndAuthN(t *testing.T) {
	t.Parallel()

	// The owners check asks GitHub about the CODEOWNERS file at OWNER_CHECKER_REF, so validating a
	// directory that isn't a Git repository is enough and keeps the test independent of the local
	// commit, which doesn't exist on GitHub.
	codeownersDir := func(t *testing.T) string {
		t.Helper()

		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "CODEOWNERS"), []byte("* @ghost\n"), 0o600))
		return dir
	}

	t.Run("token not specified", func(t *testing.T) {
		t.Parallel()

		// given
		codeownersCmd := Exec().
			Binary(os.Getenv(binaryPathEnvName)).
			WithEnv("REPOSITORY_PATH", codeownersDir(t)).
			WithEnv("CHECKS", "owners").
			WithEnv("OWNER_CHECKER_REPOSITORY", "gh-codeowners/codeowners-samples").
			WithEnv("OWNER_CHECKER_REF", "happy-path")

		// when
		result := codeownersCmd.AwaitResultAtMost(time.Minute)

		// GitHub allows 60 unauthenticated requests per hour per IP address, and hosted runners
		// share IP addresses, so the quota may already be used up.
		if strings.Contains(result.Stdout, "GitHub rate limit reached") {
			t.Skip("GitHub's unauthenticated rate limit for this IP address is exhausted")
		}

		// then
		assert.Equal(t, 0, result.ExitCode)

		normalizedOutput := normalizeTimeDurations(result.Stdout)
		g := goldie.New(t, goldie.WithNameSuffix(".golden.txt"))
		g.Assert(t, t.Name(), []byte(normalizedOutput))
	})

	t.Run("invalid token specified", func(t *testing.T) {
		t.Parallel()

		// given
		codeownersCmd := Exec().
			Binary(os.Getenv(binaryPathEnvName)).
			WithEnv("REPOSITORY_PATH", codeownersDir(t)).
			WithEnv("CHECKS", "owners").
			WithEnv("OWNER_CHECKER_REPOSITORY", "gh-codeowners/codeowners-samples").
			WithEnv("OWNER_CHECKER_REF", "happy-path").
			WithEnv("GITHUB_ACCESS_TOKEN", "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")

		// when
		result := codeownersCmd.AwaitResultAtMost(time.Minute)

		// then
		assert.Equal(t, 3, result.ExitCode)

		normalizedOutput := normalizeTimeDurations(result.Stdout)
		g := goldie.New(t, goldie.WithNameSuffix(".golden.txt"))
		g.Assert(t, t.Name(), []byte(normalizedOutput))
	})
}

// To update golden file, run:
// TEST=TestGitHubAppAuth APP_PRIVATE_KEY=`cat private-key.pem` UPDATE_GOLDEN=true make test-integration
func TestGitHubAppAuth(t *testing.T) {
	t.Parallel()

	// given
	privateKey := getEnvOrSkip(t, gitHubAppPrivateKeyEnvName)
	repoDir := SampleRepo(t, caseInsensitiveOrgCodeownersSamplesRepo, "happy-path")

	codeownersCmd := Exec().
		Binary(os.Getenv(binaryPathEnvName)).
		WithEnv("REPOSITORY_PATH", repoDir).
		WithEnv("CHECKS", "owners").
		WithEnv("OWNER_CHECKER_REPOSITORY", "GitHubCODEOWNERS/codeowners-samples").
		WithEnv("OWNER_CHECKER_REF", "happy-path").
		WithEnv("GITHUB_APP_ID", gitHubAppId).
		WithEnv("GITHUB_APP_INSTALLATION_ID", gitHubAppInstallationId).
		WithEnv("GITHUB_APP_PRIVATE_KEY", privateKey)

	// when
	result := codeownersCmd.AwaitResultAtMost(time.Minute)

	// then
	assert.Equal(t, 0, result.ExitCode)

	normalizedOutput := normalizeTimeDurations(result.Stdout)
	g := goldie.New(t, goldie.WithNameSuffix(".golden.txt"))
	g.Assert(t, t.Name(), []byte(normalizedOutput))
}

func TestMultipleChecksSuccess(t *testing.T) {
	t.Skip("not implemented yet")
}
