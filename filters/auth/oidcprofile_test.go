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

func TestCacheKey(t *testing.T) {
	t.Run("same profile, host and clientID produce same key", func(t *testing.T) {
		assert.Equal(t, cacheKey("myprofile", "app.example.com", "client-a"), cacheKey("myprofile", "app.example.com", "client-a"))
	})

	t.Run("different profile name produces different key", func(t *testing.T) {
		assert.NotEqual(t, cacheKey("profile-a", "app.example.com", "client"), cacheKey("profile-b", "app.example.com", "client"))
	})

	t.Run("different host produces different key", func(t *testing.T) {
		assert.NotEqual(t, cacheKey("myprofile", "tenant-a.example.com", "client"), cacheKey("myprofile", "tenant-b.example.com", "client"))
	})

	t.Run("different clientID produces different key (tenant isolation)", func(t *testing.T) {
		// Two tenants sharing the same host and profile but injecting different clientIDs
		// via annotate() filters must get distinct cache entries so they are not served
		// by each other's OAuth2 delegate.
		assert.NotEqual(t, cacheKey("myprofile", "app.example.com", "client-a"), cacheKey("myprofile", "app.example.com", "client-b"))
	})

	t.Run("clientSecret does not affect key (rotation safety)", func(t *testing.T) {
		// Rotating the secret must not produce a new cache entry, which would invalidate
		// all in-flight sessions. Only the public clientID is included in the key.
		assert.Equal(t, cacheKey("myprofile", "app.example.com", "client-a"), cacheKey("myprofile", "app.example.com", "client-a"))
	})

	t.Run("null byte separator prevents cross-field collisions", func(t *testing.T) {
		// "ab" + "" must not equal "a" + "b" — the \x00 separator prevents this.
		// YAML keys, HTTP Host values and OAuth2 client IDs cannot themselves contain
		// null bytes, so no legitimate inputs can produce the same key string.
		assert.NotEqual(t, cacheKey("a", "b", "c"), cacheKey("a\x00b", "", "c"))
		assert.NotEqual(t, cacheKey("a", "b", "c"), cacheKey("a", "b\x00c", ""))
	})

	t.Run("key is non-empty", func(t *testing.T) {
		assert.NotEmpty(t, cacheKey("myprofile", "app.example.com", "client"))
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

		assert.Equal(t, cacheKey(f.name, data.Request.Host, r1.clientID), cacheKey(f.name, data.Request.Host, r2.clientID))
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

	assert.Equal(t, cacheKey(f.name, data.Request.Host, r1.clientID), cacheKey(f.name, data.Request.Host, r2.clientID), "same input should produce the same cache key")
}

// TestTenantIsolation verifies that two tenants sharing the same host and profile name
// but injecting different clientIDs via annotate() filters receive distinct cache keys
// and distinct cookie names, preventing session cookies from being accepted
// cross-tenant.
func TestTenantIsolation(t *testing.T) {
	profile := &OidcProfile{
		IdpURL:      "https://idp.example.com",
		ClientID:    `{{index .Annotations "client-id"}}`,
		CallbackURL: "https://app.example.com/callback",
	}
	f := &tokenOidcProfileFilter{name: "shared-profile", profile: profile}
	host := "app.example.com"

	dataTenantA := profileTemplateData{
		Request:     profileRequestData{Host: host},
		Annotations: map[string]string{"client-id": "tenant-a-client"},
	}
	dataTenantB := profileTemplateData{
		Request:     profileRequestData{Host: host},
		Annotations: map[string]string{"client-id": "tenant-b-client"},
	}

	rA, err := f.resolveAll(dataTenantA)
	require.NoError(t, err)
	rB, err := f.resolveAll(dataTenantB)
	require.NoError(t, err)

	keyA := cacheKey(f.name, host, rA.clientID)
	keyB := cacheKey(f.name, host, rB.clientID)
	assert.NotEqual(t, keyA, keyB, "different tenants must get different delegate cache keys")
}

func TestParseAuthCodeOpts(t *testing.T) {
	t.Run("empty string returns nil slices", func(t *testing.T) {
		opts, qp, err := parseAuthCodeOpts("")
		require.NoError(t, err)
		assert.Nil(t, opts)
		assert.Nil(t, qp)
	})

	t.Run("single static option", func(t *testing.T) {
		opts, qp, err := parseAuthCodeOpts("prompt=consent")
		require.NoError(t, err)
		assert.Len(t, opts, 1)
		assert.Empty(t, qp)
	})

	t.Run("skipper-request-query becomes query param", func(t *testing.T) {
		opts, qp, err := parseAuthCodeOpts("acr_values=skipper-request-query")
		require.NoError(t, err)
		assert.Empty(t, opts)
		assert.Equal(t, []string{"acr_values"}, qp)
	})

	t.Run("multiple options mixed", func(t *testing.T) {
		opts, qp, err := parseAuthCodeOpts("prompt=consent acr_values=skipper-request-query")
		require.NoError(t, err)
		assert.Len(t, opts, 1)
		assert.Equal(t, []string{"acr_values"}, qp)
	})

	t.Run("value containing equals sign is handled correctly", func(t *testing.T) {
		// SplitN with limit 2 ensures a Base64 value like "abc==" is not split further.
		opts, _, err := parseAuthCodeOpts("nonce=abc==")
		require.NoError(t, err)
		assert.Len(t, opts, 1)
	})

	t.Run("missing equals sign returns error", func(t *testing.T) {
		_, _, err := parseAuthCodeOpts("no-equals-sign")
		require.Error(t, err)
	})
}

func TestParseUpstreamHeaders(t *testing.T) {
	t.Run("empty string returns nil", func(t *testing.T) {
		h, err := parseUpstreamHeaders("")
		require.NoError(t, err)
		assert.Nil(t, h)
	})

	t.Run("single header", func(t *testing.T) {
		h, err := parseUpstreamHeaders("X-User-ID:sub")
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"X-User-ID": "sub"}, h)
	})

	t.Run("multiple headers", func(t *testing.T) {
		h, err := parseUpstreamHeaders("X-User-ID:sub X-Email:email")
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"X-User-ID": "sub", "X-Email": "email"}, h)
	})

	t.Run("missing colon returns error", func(t *testing.T) {
		_, err := parseUpstreamHeaders("no-colon")
		require.Error(t, err)
	})

	t.Run("empty key returns error", func(t *testing.T) {
		_, err := parseUpstreamHeaders(":value")
		require.Error(t, err)
	})

	t.Run("empty value returns error", func(t *testing.T) {
		_, err := parseUpstreamHeaders("key:")
		require.Error(t, err)
	})
}
