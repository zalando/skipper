package auth_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters/auth"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/proxy/proxytest"
	"golang.org/x/oauth2"
)

const (
	testToken      = "foobarbaz"
	testAccessCode = "quxquuxquz"
)

func newTestTokeninfo(validToken string) *httptest.Server {
	const prefix = "Bearer "
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := func(code int) {
			w.WriteHeader(code)
			w.Write([]byte("{}"))
		}

		token := r.Header.Get("Authorization")
		if !strings.HasPrefix(token, prefix) || token[len(prefix):] != validToken {
			response(http.StatusUnauthorized)
			return
		}

		response(http.StatusOK)
	}))
}

func newTestAuthServer(testToken, testAccessCode string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := func(w http.ResponseWriter, r *http.Request) {
			rq := r.URL.Query()
			redirect := rq.Get("redirect_uri")
			rd, err := url.Parse(redirect)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			q := rd.Query()
			q.Set("code", testAccessCode)
			q.Set("state", r.URL.Query().Get("state"))
			rd.RawQuery = q.Encode()

			http.Redirect(
				w,
				r,
				rd.String(),
				http.StatusTemporaryRedirect,
			)
		}

		token := func(w http.ResponseWriter, r *http.Request) {
			code := r.FormValue("code")
			if code != testAccessCode {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}

			token := &oauth2.Token{
				AccessToken: testToken,
				Expiry:      time.Now().Add(time.Hour),
			}

			b, err := json.Marshal(token)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			w.Write(b)
		}

		switch r.URL.Path {
		case "/auth":
			auth(w, r)
		case "/token":
			token(w, r)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestGrantFlow(t *testing.T) {
	// create a test provider
	// create a test tokeninfo
	// create a proxy, returning 204, oauthGrant filter, initially without parameters
	// create a client without redirects, to check it manually
	// make a request to the proxy without a cookie
	// get redirected
	// get redirected
	// get redirected, check for a cookie
	// make a request to the proxy with the cookie
	// get 204

	provider := newTestAuthServer(testToken, testAccessCode)
	defer provider.Close()

	tokeninfo := newTestTokeninfo(testToken)
	defer tokeninfo.Close()

	authURL := fmt.Sprintf("%s/auth", provider.URL)
	// tokenURL := fmt.Sprintf("%s/token", provider.URL)

	spec, err := auth.NewGrant(auth.OAuthOptions{})
	if err != nil {
		t.Fatal(err)
	}

	fr := builtin.MakeRegistry()
	fr.Register(spec)
	proxy := proxytest.New(fr, &eskip.Route{
		Filters: []*eskip.Filter{
			{Name: auth.OAuthGrantName},
			{Name: "status", Args: []interface{}{http.StatusNoContent}},
		},
		BackendType: eskip.ShuntBackend,
	})

	client := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	rsp, err := client.Get(proxy.URL)
	if err != nil {
		t.Fatal(err)
	}

	if rsp.StatusCode != http.StatusTemporaryRedirect {
		t.Fatalf(
			"Unexpected status code, got: %d, expected: %d.",
			rsp.StatusCode,
			http.StatusTemporaryRedirect,
		)
	}

	if rsp.Header.Get("Location") != authURL {
		t.Fatalf(
			"Unexpected redirect location, got: '%s', expected: '%s'.",
			rsp.Header.Get("Location"),
			authURL,
		)
	}
}
