package github

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/bradleyfalzon/ghinstallation/v2"

	"github.com/mlarraz/codeowners-validator/pkg/url"

	"github.com/google/go-github/v93/github"
	"golang.org/x/oauth2"
)

type ClientConfig struct {
	AccessToken string `envconfig:"optional"`

	AppID             int64  `envconfig:"optional"`
	AppPrivateKey     string `envconfig:"optional"`
	AppInstallationID int64  `envconfig:"optional"`

	BaseURL            string        `envconfig:"optional"`
	UploadURL          string        `envconfig:"optional"`
	HTTPRequestTimeout time.Duration `envconfig:"default=30s"`
}

// Validate validates if provided client options are valid.
//
// Credentials are optional, so that public repositories can be validated without
// authentication. When both a token and a GitHub App are configured, the App is used.
func (c *ClientConfig) Validate() error {
	if c.AppID != 0 {
		if c.AppInstallationID == 0 {
			return errors.New("GitHub APP_INSTALLATION_ID is required with APP_ID")
		}
		if c.AppPrivateKey == "" {
			return errors.New("GitHub APP_PRIVATE_KEY is required with APP_ID")
		}
	}

	return nil
}

func NewClient(ctx context.Context, cfg *ClientConfig) (*github.Client, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	httpClient := http.DefaultClient
	var err error

	if cfg.AppID != 0 {
		httpClient, err = createAppInstallationHTTPClient(cfg)
		if err != nil {
			return nil, err
		}
	} else if cfg.AccessToken != "" {
		httpClient = oauth2.NewClient(ctx, oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: cfg.AccessToken},
		))
	}

	opts := []github.ClientOptionsFunc{
		github.WithHTTPClient(httpClient),
		github.WithTimeout(cfg.HTTPRequestTimeout),
	}

	if baseURL, uploadURL := cfg.BaseURL, cfg.UploadURL; baseURL != "" {
		if uploadURL == "" { // often the baseURL is same as the uploadURL, so we do not require to provide both of them
			uploadURL = baseURL
		}
		opts = append(opts, github.WithEnterpriseURLs(url.CanonicalPath(baseURL), url.CanonicalPath(uploadURL)))
	}

	ghClient, err := github.NewClient(opts...)
	if err != nil {
		return nil, err
	}
	return ghClient, nil
}

func createAppInstallationHTTPClient(cfg *ClientConfig) (client *http.Client, err error) {
	tr := http.DefaultTransport
	itr, err := ghinstallation.New(tr, cfg.AppID, cfg.AppInstallationID, []byte(cfg.AppPrivateKey))
	if err != nil {
		return nil, err
	}

	return &http.Client{Transport: itr}, nil
}
