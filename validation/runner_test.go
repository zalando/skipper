package validation

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/skipper/dataclients/kubernetes/definitions"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/block"
	"github.com/zalando/skipper/predicates/methods"
	"github.com/zalando/skipper/routing"
)

func TestRunnerRequiresTLS(t *testing.T) {
	patchLogrusExit(t)

	runner := NewRunner(nil)
	err := runner.StartValidation(":0", "", "", nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "requires TLS")
}

func TestRunnerServesAdmissionHandlers(t *testing.T) {
	patchLogrusExit(t)

	testCases := []struct {
		name            string
		path            string
		payload         map[string]any
		expectedAllowed bool
		expectedMessage string
	}{
		{
			name: "routegroup filter validation success",
			path: "/routegroups",
			payload: newRouteGroupPayload(func(spec map[string]any) {
				spec["routes"] = []map[string]any{
					{"filters": []string{"blockContent(\"abc\")"}},
				}
			}),
			expectedAllowed: true,
		},
		{
			name: "routegroup filter validation error",
			path: "/routegroups",
			payload: newRouteGroupPayload(func(spec map[string]any) {
				spec["routes"] = []map[string]any{
					{"filters": []string{"blockContent()"}},
				}
			}),
			expectedMessage: "invalid filter parameters",
		},
		{
			name: "routegroup unknown filter validation error",
			path: "/routegroups",
			payload: newRouteGroupPayload(func(spec map[string]any) {
				spec["routes"] = []map[string]any{
					{"filters": []string{"unknownFilter()"}},
				}
			}),
			expectedMessage: "filter \"unknownFilter\" not found",
		},
		{
			name: "routegroup predicate validation success",
			path: "/routegroups",
			payload: newRouteGroupPayload(func(spec map[string]any) {
				spec["routes"] = []map[string]any{
					{"predicates": []string{"Methods(\"GET\")"}},
				}
			}),
			expectedAllowed: true,
		},
		{
			name: "routegroup predicate validation error",
			path: "/routegroups",
			payload: newRouteGroupPayload(func(spec map[string]any) {
				spec["routes"] = []map[string]any{
					{"predicates": []string{"Methods()"}},
				}
			}),
			expectedMessage: "at least one method should be specified",
		},
		{
			name: "routegroup unknown predicate validation error",
			path: "/routegroups",
			payload: newRouteGroupPayload(func(spec map[string]any) {
				spec["routes"] = []map[string]any{
					{"predicates": []string{"UnknownPredicate()"}},
				}
			}),
			expectedMessage: "predicate \"UnknownPredicate\" not found",
		},
		{
			name: "routegroup backend validation error",
			path: "/routegroups",
			payload: newRouteGroupPayload(func(spec map[string]any) {
				spec["backends"] = []map[string]any{
					{"name": "backend-1", "type": "network", "address": "example.com"},
				}
			}),
			expectedMessage: "backend address",
		},
		{
			name: "ingress predicate annotation validation success",
			path: "/ingresses",
			payload: newIngressPayload(func(meta map[string]any) {
				annotations := meta["annotations"].(map[string]any)
				annotations[definitions.IngressPredicateAnnotation] = "Methods(\"GET\")"
			}),
			expectedAllowed: true,
		},
		{
			name: "ingress predicate annotation validation error",
			path: "/ingresses",
			payload: newIngressPayload(func(meta map[string]any) {
				annotations := meta["annotations"].(map[string]any)
				annotations[definitions.IngressPredicateAnnotation] = "Methods()"
			}),
			expectedMessage: "at least one method should be specified",
		},
		{
			name: "ingress predicate annotation unknown predicate",
			path: "/ingresses",
			payload: newIngressPayload(func(meta map[string]any) {
				annotations := meta["annotations"].(map[string]any)
				annotations[definitions.IngressPredicateAnnotation] = "UnknownPredicate()"
			}),
			expectedMessage: "predicate \"UnknownPredicate\" not found",
		},
		{
			name: "ingress filter annotation validation success",
			path: "/ingresses",
			payload: newIngressPayload(func(meta map[string]any) {
				annotations := meta["annotations"].(map[string]any)
				annotations[definitions.IngressFilterAnnotation] = "blockContent(\"abc\")"
			}),
			expectedAllowed: true,
		},
		{
			name: "ingress filter annotation validation error",
			path: "/ingresses",
			payload: newIngressPayload(func(meta map[string]any) {
				annotations := meta["annotations"].(map[string]any)
				annotations[definitions.IngressFilterAnnotation] = "blockContent()"
			}),
			expectedMessage: "invalid filter parameters",
		},
		{
			name: "ingress filter annotation unknown filter",
			path: "/ingresses",
			payload: newIngressPayload(func(meta map[string]any) {
				annotations := meta["annotations"].(map[string]any)
				annotations[definitions.IngressFilterAnnotation] = "unknownFilter()"
			}),
			expectedMessage: "filter \"unknownFilter\" not found",
		},
		{
			name: "ingress routes annotation validation success",
			path: "/ingresses",
			payload: newIngressPayload(func(meta map[string]any) {
				annotations := meta["annotations"].(map[string]any)
				annotations[definitions.IngressRoutesAnnotation] = "r1: * -> blockContent(\"abc\") -> \"https://example.org\""
			}),
			expectedAllowed: true,
		},
		{
			name: "ingress routes annotation validation error",
			path: "/ingresses",
			payload: newIngressPayload(func(meta map[string]any) {
				annotations := meta["annotations"].(map[string]any)
				annotations[definitions.IngressRoutesAnnotation] = "r1: * -> blockContent() -> \"https://example.org\""
			}),
			expectedMessage: "invalid filter parameters",
		},
		{
			name: "ingress routes annotation unknown filter",
			path: "/ingresses",
			payload: newIngressPayload(func(meta map[string]any) {
				annotations := meta["annotations"].(map[string]any)
				annotations[definitions.IngressRoutesAnnotation] = "r1: * -> unknownFilter() -> \"https://example.org\""
			}),
			expectedMessage: "filter \"unknownFilter\" not found",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			certFile, keyFile := generateSelfSignedCert(t)
			address, addrErr := freeAddress()
			if addrErr != nil {
				if isPermissionDenied(addrErr) {
					t.Skipf("skipping, cannot bind test socket: %v", addrErr)
				}
				require.NoError(t, addrErr)
			}

			filterRegistry := filters.Registry{}
			filterRegistry.Register(block.NewBlock(1024))

			predicateSpecs := []routing.PredicateSpec{
				methods.New(),
			}

			runner := NewRunner(nil)

			err := runner.StartValidation(address, certFile, keyFile, filterRegistry, predicateSpecs)
			if isPermissionDenied(err) {
				t.Skipf("skipping, cannot bind validation listener: %v", err)
			}
			require.NoError(t, err)

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			t.Cleanup(func() {
				cancel()
				_ = runner.Stop(context.Background())
			})

			transport := &http.Transport{
				Proxy: func(*http.Request) (*url.URL, error) { // bypass proxies so local test endpoints stay reachable
					return nil, nil
				},
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
			}
			t.Cleanup(transport.CloseIdleConnections)
			client := &http.Client{
				Timeout:   time.Second,
				Transport: transport,
			}

			require.Eventually(t, func() bool {
				resp, err := client.Get("https://" + address + "/healthz")
				if err != nil {
					return false
				}
				resp.Body.Close()
				return resp.StatusCode == http.StatusOK
			}, 2*time.Second, 50*time.Millisecond)

			body, err := json.Marshal(tc.payload)
			require.NoError(t, err)

			resp, err := client.Post("https://"+address+tc.path, "application/json", bytes.NewReader(body))
			require.NoError(t, err)
			defer resp.Body.Close()
			assert.Equal(t, http.StatusOK, resp.StatusCode)

			var review struct {
				Response struct {
					Allowed bool `json:"allowed"`
					Status  struct {
						Message string `json:"message"`
					} `json:"status"`
				} `json:"response"`
			}

			require.NoError(t, json.NewDecoder(resp.Body).Decode(&review))
			assert.Equal(t, tc.expectedAllowed, review.Response.Allowed)
			if tc.expectedMessage != "" {
				assert.Contains(t, review.Response.Status.Message, tc.expectedMessage)
			} else {
				assert.Empty(t, review.Response.Status.Message)
			}

			require.NoError(t, runner.Stop(ctx))
		})
	}
}

func patchLogrusExit(t *testing.T) {
	t.Helper()
	logger := log.StandardLogger()
	original := logger.ExitFunc
	logger.ExitFunc = func(int) {}
	t.Cleanup(func() {
		logger.ExitFunc = original
	})
}

func freeAddress() (string, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	addr := ln.Addr().String()
	if err := ln.Close(); err != nil {
		return "", err
	}
	return addr, nil
}

func generateSelfSignedCert(t *testing.T) (string, string) {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:     []string{"localhost"},
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, template, template, &priv.PublicKey, priv)
	require.NoError(t, err)

	dir := t.TempDir()
	certFile := filepath.Join(dir, "cert.pem")
	keyFile := filepath.Join(dir, "key.pem")

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})

	require.NoError(t, os.WriteFile(certFile, certPEM, 0o600))
	require.NoError(t, os.WriteFile(keyFile, keyPEM, 0o600))

	return certFile, keyFile
}

func init() {
	// Reduce noise from logrus during tests.
	log.SetFormatter(&log.TextFormatter{DisableTimestamp: true})
	log.SetLevel(log.WarnLevel)
}

func isPermissionDenied(err error) bool {
	if err == nil {
		return false
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		err = opErr.Err
	}
	return errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES)
}

func newRouteGroupPayload(modifier func(spec map[string]any)) map[string]any {
	spec := map[string]any{
		"backends": []map[string]any{
			{"name": "backend-1", "type": "network", "address": "https://example.org"},
		},
		"defaultBackends": []map[string]any{
			{"backendName": "backend-1"},
		},
		"routes": []map[string]any{},
	}
	if modifier != nil {
		modifier(spec)
	}

	return map[string]any{
		"request": map[string]any{
			"uid":       "req-uid",
			"name":      "rg-test",
			"namespace": "ns-test",
			"resource": map[string]any{
				"group":    "zalando.org",
				"version":  "v1",
				"resource": "routegroups",
			},
			"object": map[string]any{
				"metadata": map[string]any{
					"name":      "rg-test",
					"namespace": "ns-test",
				},
				"spec": spec,
			},
		},
	}
}

func newIngressPayload(modifier func(metadata map[string]any)) map[string]any {
	metadata := map[string]any{
		"name":        "ing-test",
		"namespace":   "ns-test",
		"annotations": map[string]any{},
	}
	if modifier != nil {
		modifier(metadata)
	}

	paths := []map[string]any{
		{
			"path":     "/",
			"pathType": "Prefix",
			"backend": map[string]any{
				"service": map[string]any{
					"name": "example-service",
					"port": map[string]any{"number": 80},
				},
			},
		},
	}

	rules := []map[string]any{
		{
			"host": "example.com",
			"http": map[string]any{
				"paths": paths,
			},
		},
	}

	object := map[string]any{
		"metadata": metadata,
		"spec": map[string]any{
			"rules": rules,
		},
	}

	return map[string]any{
		"request": map[string]any{
			"uid":       "req-uid",
			"name":      "ing-test",
			"namespace": "ns-test",
			"resource": map[string]any{
				"group":    "networking.k8s.io",
				"version":  "v1",
				"resource": "ingresses",
			},
			"object": object,
		},
	}
}
