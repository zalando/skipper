package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOidcProfileValidate(t *testing.T) {
	t.Run("valid static profile", func(t *testing.T) {
		p := &OidcProfile{
			IdpURL:      "https://idp.example.com",
			ClientID:    "client-id",
			CallbackURL: "https://app.example.com/callback",
		}
		assert.NoError(t, p.Validate())
	})

	t.Run("valid profile with templates", func(t *testing.T) {
		p := &OidcProfile{
			IdpURL:      "https://idp.example.com",
			ClientID:    `{{index .Annotations "client-id"}}`,
			CallbackURL: `https://{{.Request.Host}}/callback`,
		}
		assert.NoError(t, p.Validate())
	})

	t.Run("empty IdpURL is rejected", func(t *testing.T) {
		p := &OidcProfile{
			ClientID:    "client-id",
			CallbackURL: "https://app.example.com/callback",
		}
		err := p.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "IdpURL is required")
	})

	t.Run("invalid template in ClientID", func(t *testing.T) {
		p := &OidcProfile{
			IdpURL:   "https://idp.example.com",
			ClientID: "{{unclosed",
		}
		err := p.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "template parse error")
	})

	t.Run("invalid template in CallbackURL", func(t *testing.T) {
		p := &OidcProfile{
			IdpURL:      "https://idp.example.com",
			ClientID:    "client-id",
			CallbackURL: "{{broken template",
		}
		err := p.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "template parse error")
	})

	t.Run("invalid template in CookieName", func(t *testing.T) {
		p := &OidcProfile{
			IdpURL:     "https://idp.example.com",
			ClientID:   "client-id",
			CookieName: "{{.Bad",
		}
		err := p.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "template parse error")
	})
}

func TestResolveField(t *testing.T) {
	data := profileTemplateData{
		Request:     profileRequestData{Host: "myapp.example.com"},
		Annotations: map[string]string{"tenant": "acme", "env": "prod"},
	}

	t.Run("empty string returns empty", func(t *testing.T) {
		result, err := resolveField("", data)
		require.NoError(t, err)
		assert.Equal(t, "", result)
	})

	t.Run("static string passes through", func(t *testing.T) {
		result, err := resolveField("https://idp.example.com", data)
		require.NoError(t, err)
		assert.Equal(t, "https://idp.example.com", result)
	})

	t.Run("request host template", func(t *testing.T) {
		result, err := resolveField("https://{{.Request.Host}}/callback", data)
		require.NoError(t, err)
		assert.Equal(t, "https://myapp.example.com/callback", result)
	})

	t.Run("annotation lookup template", func(t *testing.T) {
		result, err := resolveField(`{{index .Annotations "tenant"}}`, data)
		require.NoError(t, err)
		assert.Equal(t, "acme", result)
	})

	t.Run("missing annotation returns empty string", func(t *testing.T) {
		result, err := resolveField(`{{index .Annotations "missing"}}`, data)
		require.NoError(t, err)
		assert.Equal(t, "", result)
	})

	t.Run("combined template", func(t *testing.T) {
		result, err := resolveField(`client-{{index .Annotations "tenant"}}-{{index .Annotations "env"}}`, data)
		require.NoError(t, err)
		assert.Equal(t, "client-acme-prod", result)
	})

	t.Run("invalid template syntax returns error", func(t *testing.T) {
		_, err := resolveField("{{unclosed", data)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "template parse error")
	})
}

func TestResolvedProfileCacheKey(t *testing.T) {
	t.Run("same fields produce same key", func(t *testing.T) {
		r1 := &resolvedProfile{
			clientID:     "client-a",
			clientSecret: "secret-a",
			callbackURL:  "https://app.example.com/callback",
			scopes:       "email profile",
		}
		r2 := &resolvedProfile{
			clientID:     "client-a",
			clientSecret: "secret-a",
			callbackURL:  "https://app.example.com/callback",
			scopes:       "email profile",
		}
		assert.Equal(t, r1.cacheKey(), r2.cacheKey())
	})

	t.Run("different clientID produces different key", func(t *testing.T) {
		r1 := &resolvedProfile{clientID: "client-a"}
		r2 := &resolvedProfile{clientID: "client-b"}
		assert.NotEqual(t, r1.cacheKey(), r2.cacheKey())
	})

	t.Run("different scopes produces different key", func(t *testing.T) {
		r1 := &resolvedProfile{clientID: "c", scopes: "email"}
		r2 := &resolvedProfile{clientID: "c", scopes: "email profile"}
		assert.NotEqual(t, r1.cacheKey(), r2.cacheKey())
	})

	t.Run("key is non-empty hex string", func(t *testing.T) {
		r := &resolvedProfile{clientID: "c", clientSecret: "s"}
		key := r.cacheKey()
		assert.NotEmpty(t, key)
		// SHA-256 hex = 64 chars
		assert.Len(t, key, 64)
	})

	t.Run("field order matters - clientID vs cookieName not interchangeable", func(t *testing.T) {
		r1 := &resolvedProfile{clientID: "value", cookieName: ""}
		r2 := &resolvedProfile{clientID: "", cookieName: "value"}
		assert.NotEqual(t, r1.cacheKey(), r2.cacheKey())
	})
}

func TestTokenOidcProfileFilterResolveAll(t *testing.T) {
	t.Run("static profile resolves to same values", func(t *testing.T) {
		profile := &OidcProfile{
			IdpURL:       "https://idp.example.com",
			ClientID:     "static-client",
			ClientSecret: "static-secret",
			CallbackURL:  "https://app.example.com/callback",
			Scopes:       "email profile",
		}
		f := &tokenOidcProfileFilter{profile: profile}
		data := profileTemplateData{
			Request:     profileRequestData{Host: "app.example.com"},
			Annotations: map[string]string{},
		}
		r, err := f.resolveAll(data)
		require.NoError(t, err)
		assert.Equal(t, "static-client", r.clientID)
		assert.Equal(t, "static-secret", r.clientSecret)
		assert.Equal(t, "https://app.example.com/callback", r.callbackURL)
		assert.Equal(t, "email profile", r.scopes)
	})

	t.Run("annotation templates resolved from data", func(t *testing.T) {
		profile := &OidcProfile{
			IdpURL:       "https://idp.example.com",
			ClientID:     `{{index .Annotations "my-client-id"}}`,
			ClientSecret: `{{index .Annotations "my-client-secret"}}`,
			CallbackURL:  `https://{{.Request.Host}}/auth/callback`,
		}
		f := &tokenOidcProfileFilter{profile: profile}
		data := profileTemplateData{
			Request: profileRequestData{Host: "tenant.example.com"},
			Annotations: map[string]string{
				"my-client-id":     "tenant-client",
				"my-client-secret": "tenant-secret",
			},
		}
		r, err := f.resolveAll(data)
		require.NoError(t, err)
		assert.Equal(t, "tenant-client", r.clientID)
		assert.Equal(t, "tenant-secret", r.clientSecret)
		assert.Equal(t, "https://tenant.example.com/auth/callback", r.callbackURL)
	})

	t.Run("different annotations produce different resolved profiles", func(t *testing.T) {
		profile := &OidcProfile{
			IdpURL:   "https://idp.example.com",
			ClientID: `{{index .Annotations "client-id"}}`,
		}
		f := &tokenOidcProfileFilter{profile: profile}

		data1 := profileTemplateData{
			Annotations: map[string]string{"client-id": "client-a"},
		}
		data2 := profileTemplateData{
			Annotations: map[string]string{"client-id": "client-b"},
		}

		r1, err := f.resolveAll(data1)
		require.NoError(t, err)
		r2, err := f.resolveAll(data2)
		require.NoError(t, err)

		assert.NotEqual(t, r1.clientID, r2.clientID)
		assert.NotEqual(t, r1.cacheKey(), r2.cacheKey())
	})

	t.Run("same annotations produce same cache key", func(t *testing.T) {
		profile := &OidcProfile{
			IdpURL:      "https://idp.example.com",
			ClientID:    `{{index .Annotations "client-id"}}`,
			CallbackURL: `https://{{.Request.Host}}/callback`,
		}
		f := &tokenOidcProfileFilter{profile: profile}

		data := profileTemplateData{
			Request:     profileRequestData{Host: "app.example.com"},
			Annotations: map[string]string{"client-id": "my-client"},
		}

		r1, err := f.resolveAll(data)
		require.NoError(t, err)
		r2, err := f.resolveAll(data)
		require.NoError(t, err)

		assert.Equal(t, r1.cacheKey(), r2.cacheKey())
	})

	t.Run("subdomains to remove field resolved", func(t *testing.T) {
		profile := &OidcProfile{
			IdpURL:             "https://idp.example.com",
			SubdomainsToRemove: "2",
		}
		f := &tokenOidcProfileFilter{profile: profile}
		data := profileTemplateData{}
		r, err := f.resolveAll(data)
		require.NoError(t, err)
		assert.Equal(t, "2", r.subdomainsToRemove)
	})

	t.Run("auth code opts and upstream headers resolved", func(t *testing.T) {
		profile := &OidcProfile{
			IdpURL:          "https://idp.example.com",
			AuthCodeOpts:    "prompt=consent",
			UpstreamHeaders: "X-User-ID:sub",
		}
		f := &tokenOidcProfileFilter{profile: profile}
		data := profileTemplateData{}
		r, err := f.resolveAll(data)
		require.NoError(t, err)
		assert.Equal(t, "prompt=consent", r.authCodeOpts)
		assert.Equal(t, "X-User-ID:sub", r.upstreamHeaders)
	})
}

func TestOidcProfileDelegateCaching(t *testing.T) {
	// This test verifies that calling resolveAll twice with the same data
	// produces the same cache key (the foundation for delegate caching).
	profile := &OidcProfile{
		IdpURL:       "https://idp.example.com",
		ClientID:     `{{index .Annotations "cid"}}`,
		ClientSecret: "secret",
		CallbackURL:  "https://app.example.com/cb",
	}
	f := &tokenOidcProfileFilter{profile: profile}

	data := profileTemplateData{
		Annotations: map[string]string{"cid": "my-client"},
	}

	r1, err := f.resolveAll(data)
	require.NoError(t, err)
	r2, err := f.resolveAll(data)
	require.NoError(t, err)

	assert.Equal(t, r1.cacheKey(), r2.cacheKey(), "same input should produce the same cache key")
}
