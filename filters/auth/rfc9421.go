package auth

import (
	"crypto"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net/http"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/rfc/rfc9421"
)

const (
	RFC9421Name      = "rfc9421"
	privateKeyHeader = "x-rfc9421-private-key"
)

type rfc9421Spec struct{}

type rfc9421Filter struct {
	keyID      string
	algorithm  string
	components []string
	signLabel  string
}

// NewRFC9421 creates a new filter specification for RFC 9421 HTTP Message Signatures.
func NewRFC9421() filters.Spec {
	return &rfc9421Spec{}
}

func (*rfc9421Spec) Name() string {
	return RFC9421Name
}

func (*rfc9421Spec) CreateFilter(args []interface{}) (filters.Filter, error) {
	if len(args) < 3 || len(args) > 4 {
		return nil, filters.ErrInvalidFilterParameters
	}

	keyID, ok := args[0].(string)
	if !ok || keyID == "" {
		return nil, filters.ErrInvalidFilterParameters
	}

	algorithm, ok := args[1].(string)
	if !ok || algorithm == "" {
		return nil, filters.ErrInvalidFilterParameters
	}

	componentsStr, ok := args[2].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}

	var components []string
	for _, c := range strings.Split(componentsStr, ",") {
		trimmed := strings.TrimSpace(c)
		if trimmed != "" {
			components = append(components, trimmed)
		}
	}

	signLabel := "sig1"
	if len(args) == 4 {
		labelArg, ok := args[3].(string)
		if !ok || strings.TrimSpace(labelArg) == "" {
			return nil, filters.ErrInvalidFilterParameters
		}
		signLabel = strings.TrimSpace(labelArg)
	}

	return &rfc9421Filter{
		keyID:      keyID,
		algorithm:  algorithm,
		components: components,
		signLabel:  signLabel,
	}, nil
}

func (f *rfc9421Filter) Request(ctx filters.FilterContext) {
	req := ctx.Request()
	logger := log.WithContext(req.Context())

	keyRaw := getAndRemoveHeader(privateKeyHeader, req)
	if keyRaw == "" {
		logger.Warnf("rfc9421: %s header is missing", privateKeyHeader)
		return
	}

	privKey, err := parsePrivateKey(keyRaw, f.algorithm)
	if err != nil {
		logger.Errorf("rfc9421: failed to parse private key: %v", err)
		return
	}

	signer, err := rfc9421.NewSigner(rfc9421.SignerConfig{
		KeyID:      f.keyID,
		Algorithm:  f.algorithm,
		PrivateKey: privKey,
		Components: f.components,
		SignLabel:  f.signLabel,
	})
	if err != nil {
		logger.Errorf("rfc9421: failed to create signer: %v", err)
		return
	}

	sigInput, sig, err := signer.SignRequest(req)
	if err != nil {
		logger.Errorf("rfc9421: failed to sign request: %v", err)
		return
	}

	req.Header.Set("Signature-Input", sigInput)
	req.Header.Set("Signature", sig)
}

func (f *rfc9421Filter) Response(ctx filters.FilterContext) {}

func getAndRemoveHeader(headerName string, req *http.Request) string {
	val := req.Header.Get(headerName)
	if val != "" {
		req.Header.Del(headerName)
	}
	return val
}

func parsePrivateKey(rawKey string, alg string) (crypto.PrivateKey, error) {
	if alg == rfc9421.AlgorithmHmacSha256 {
		return []byte(rawKey), nil
	}

	block, _ := pem.Decode([]byte(rawKey))
	var der []byte
	if block != nil {
		der = block.Bytes
	} else {
		der = []byte(rawKey)
	}

	// Try PKCS#8
	if parsedKey, err := x509.ParsePKCS8PrivateKey(der); err == nil {
		return parsedKey, nil
	}

	// Try PKCS#1 RSA
	if rsaKey, err := x509.ParsePKCS1PrivateKey(der); err == nil {
		return rsaKey, nil
	}

	// Try EC Private Key
	if ecKey, err := x509.ParseECPrivateKey(der); err == nil {
		return ecKey, nil
	}

	// Try raw Ed25519 seed or key
	if alg == rfc9421.AlgorithmEd25519 && (len(der) == ed25519.PrivateKeySize || len(der) == ed25519.SeedSize) {
		if len(der) == ed25519.SeedSize {
			return ed25519.NewKeyFromSeed(der), nil
		}
		return ed25519.PrivateKey(der), nil
	}

	return nil, errors.New("unsupported or invalid private key format")
}
