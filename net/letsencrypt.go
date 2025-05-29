package net

import (
	"crypto/tls"
	"net"
	"net/http"
	"regexp"
	"strings"

	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
)

type letsencrypt struct {
	manager *autocert.Manager
}

func NewLetsencrypt(t bool, cache autocert.Cache, email string, domains []string) *letsencrypt {
	for _, s := range domains {
		if validateDomain(s) {
			domains = append(domains, s)
		}
	}

	manager := &autocert.Manager{
		Cache:      cache,
		Email:      email,
		HostPolicy: autocert.HostWhitelist(domains...),
		Prompt:     autocert.AcceptTOS,
	}
	if t {
		manager.Client = &acme.Client{
			DirectoryURL: "https://acme-staging-v02.api.letsencrypt.org/directory",
			UserAgent:    "skipper-test",
			HTTPClient:   http.DefaultClient,
		}
	}

	return &letsencrypt{
		manager: manager,
	}
}

func (le *letsencrypt) TLSConfig() *tls.Config {
	return le.manager.TLSConfig()
}

// Listener returns a net.Listener that need to be closed on exit or
// you leak a goroutine
func (le *letsencrypt) Listener() net.Listener {
	return le.manager.Listener()
}

func (le *letsencrypt) Client() *acme.Client {
	return le.manager.Client
}

func (le *letsencrypt) Close() {
	le.Listener().Close()
}

func validateDomain(s string) bool {
	matchDomainPart, err := regexp.Compile("^[a-z0-9]+$")
	if err != nil {
		return false
	}

	i := 0
	for _, w := range strings.Split(s, ".") {
		if !matchDomainPart.MatchString(w) {
			return false
		}
		i++
	}
	return i > 1
}
