package auth_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/skipper/filters/auth"
	"github.com/zalando/skipper/secrets"
)

func TestOAuthConfigInit(t *testing.T) {
	tempDir := t.TempDir()
	missingIDFile := tempDir + "/missing-id"
	missingSecretFile := tempDir + "/missing-secret"
	existingFile := tempDir + "/existing"
	require.NoError(t, os.WriteFile(existingFile, []byte("acontent"), 0644))

	for i, tc := range []struct {
		config        *auth.OAuthConfig
		expectedError string
	}{
		{
			&auth.OAuthConfig{
				TokeninfoURL: "https://foo.test",
				AuthURL:      "https://foo.test",
				TokenURL:     "https://foo.test",
				Secrets:      secrets.NewRegistry(),
				SecretFile:   existingFile,

				ClientIDFile:     existingFile,
				ClientSecretFile: existingFile,
			},
			"missing secrets provider",
		},
		{
			&auth.OAuthConfig{
				TokeninfoURL: "https://foo.test",
				AuthURL:      "https://foo.test",
				TokenURL:     "https://foo.test",
				Secrets:      secrets.NewRegistry(),
				SecretFile:   existingFile,

				ClientID:         "client-id",
				ClientSecretFile: existingFile,
			},
			"missing secrets provider",
		},
		{
			&auth.OAuthConfig{
				TokeninfoURL: "https://foo.test",
				AuthURL:      "https://foo.test",
				TokenURL:     "https://foo.test",
				Secrets:      secrets.NewRegistry(),
				SecretFile:   existingFile,

				SecretsProvider:  secrets.NewSecretPaths(0),
				ClientIDFile:     missingIDFile,
				ClientSecretFile: missingSecretFile,
			},
			fmt.Sprintf("lstat %s: no such file or directory", missingIDFile),
		},
		{
			&auth.OAuthConfig{
				TokeninfoURL: "https://foo.test",
				AuthURL:      "https://foo.test",
				TokenURL:     "https://foo.test",
				Secrets:      secrets.NewRegistry(),
				SecretFile:   existingFile,

				SecretsProvider:  secrets.NewSecretPaths(0),
				ClientIDFile:     existingFile,
				ClientSecretFile: missingSecretFile,
			},
			fmt.Sprintf("lstat %s: no such file or directory", missingSecretFile),
		},
	} {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			t.Cleanup(func() {
				c := tc.config
				if c.Secrets != nil {
					c.Secrets.Close()
				}
				if c.SecretsProvider != nil {
					c.SecretsProvider.Close()
				}
				if c.TokeninfoClient != nil {
					c.TokeninfoClient.Close()
				}
				if c.AuthClient != nil {
					c.AuthClient.Close()
				}
			})

			err := tc.config.Init()
			assert.EqualError(t, err, tc.expectedError)
		})
	}
}
