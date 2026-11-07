package github_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gh "github.com/mlarraz/codeowners-validator/internal/github"
)

func TestClientConfigValidate(t *testing.T) {
	const privateKey = "private-key"

	tests := map[string]struct {
		cfg     gh.ClientConfig
		wantErr string
	}{
		"No credentials, unauthenticated access to public repositories": {
			cfg: gh.ClientConfig{},
		},
		"Access token only": {
			cfg: gh.ClientConfig{AccessToken: "token"},
		},
		"GitHub App only": {
			cfg: gh.ClientConfig{AppID: 1, AppInstallationID: 2, AppPrivateKey: privateKey},
		},
		"Access token and GitHub App": {
			cfg: gh.ClientConfig{AccessToken: "token", AppID: 1, AppInstallationID: 2, AppPrivateKey: privateKey},
		},
		"GitHub App without installation ID": {
			cfg:     gh.ClientConfig{AppID: 1, AppPrivateKey: privateKey},
			wantErr: "APP_INSTALLATION_ID is required",
		},
		"GitHub App without private key": {
			cfg:     gh.ClientConfig{AppID: 1, AppInstallationID: 2},
			wantErr: "APP_PRIVATE_KEY is required",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			err := tc.cfg.Validate()
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestNewClientWithoutCredentials(t *testing.T) {
	// given
	var gotAuthorization []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v3/repos/org/repo", r.URL.Path)
		gotAuthorization = append(gotAuthorization, r.Header.Get("Authorization"))

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id": 1, "name": "repo"}`))
	}))
	t.Cleanup(srv.Close)

	cfg := gh.ClientConfig{BaseURL: srv.URL, HTTPRequestTimeout: 5 * time.Second}

	// when
	client, err := gh.NewClient(context.Background(), &cfg)
	require.NoError(t, err)

	repo, _, err := client.Repositories.Get(context.Background(), "org", "repo")

	// then
	require.NoError(t, err)
	assert.Equal(t, "repo", repo.GetName())
	assert.Equal(t, []string{""}, gotAuthorization, "no credentials must mean an unauthenticated client")
}

func TestNewClientPrefersGitHubAppOverAccessToken(t *testing.T) {
	cfg := gh.ClientConfig{
		AccessToken:       "static-token",
		AppID:             1,
		AppInstallationID: 2,
		AppPrivateKey:     "not-a-private-key",
	}

	// when
	_, err := gh.NewClient(context.Background(), &cfg)

	// then
	require.Error(t, err, "the GitHub App credentials must be used, so the invalid private key is reported")
	assert.Contains(t, err.Error(), "private key")
}
