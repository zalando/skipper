//go:build awsnitro && linux

package nitro

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/acm"
	"github.com/stretchr/testify/require"
)

type testACM struct {
	output *acm.GetCertificateOutput
	err    error
}

func (c *testACM) GetCertificate(context.Context, *acm.GetCertificateInput, ...func(*acm.Options)) (*acm.GetCertificateOutput, error) {
	return c.output, c.err
}

type testSigner struct {
	publicKey crypto.PublicKey
	destroyed int
}

func (s *testSigner) Public() crypto.PublicKey {
	return s.publicKey
}

func (s *testSigner) Sign(io.Reader, []byte, crypto.SignerOpts) ([]byte, error) {
	return nil, errors.New("test signer")
}

func (s *testSigner) Destroy() error {
	s.destroyed++
	return nil
}

func TestRefreshAndClose(t *testing.T) {
	firstPEM, firstCert := testCertificate(t, 1)
	secondPEM, secondCert := testCertificate(t, 2)
	client := &testACM{output: &acm.GetCertificateOutput{Certificate: &firstPEM}}
	var signers []*testSigner
	p := &provider{
		client: client,
		config: Config{CertificateARN: "arn:test"},
		newSigner: func(_ Config, publicKey crypto.PublicKey) (signerPool, error) {
			signer := &testSigner{publicKey: publicKey}
			signers = append(signers, signer)
			return signer, nil
		},
	}

	require.NoError(t, p.refresh(context.Background()))
	first, err := p.GetCertificate(nil)
	require.NoError(t, err)
	require.Equal(t, firstCert.Raw, first.Leaf.Raw)
	require.Len(t, signers, 1)

	client.output = &acm.GetCertificateOutput{Certificate: &secondPEM}
	require.NoError(t, p.refresh(context.Background()))
	second, err := p.GetCertificate(nil)
	require.NoError(t, err)
	require.Equal(t, secondCert.Raw, second.Leaf.Raw)
	require.Len(t, signers, 2)

	client.err = errors.New("ACM unavailable")
	require.Error(t, p.refresh(context.Background()))
	current, err := p.GetCertificate(nil)
	require.NoError(t, err)
	require.Same(t, second, current)

	p.cancel = func() {}
	p.refreshDone = make(chan struct{})
	close(p.refreshDone)
	require.NoError(t, p.Close())
	require.NoError(t, p.Close())
	require.Equal(t, 1, signers[0].destroyed)
	require.Equal(t, 1, signers[1].destroyed)
}

func TestParseCertificateChain(t *testing.T) {
	leafPEM, leaf := testCertificate(t, 1)
	chainPEM, chainCert := testCertificate(t, 2)
	chainDER, parsed, err := parseCertificateChain(leafPEM, &chainPEM)
	require.NoError(t, err)
	require.Equal(t, [][]byte{leaf.Raw, chainCert.Raw}, chainDER)
	require.Equal(t, []*x509.Certificate{leaf, chainCert}, parsed)

	_, _, err = parseCertificateChain("not PEM", nil)
	require.Error(t, err)
}

func testCertificate(t *testing.T, serial int64) (string, *x509.Certificate) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(serial),
		Subject:               pkix.Name{CommonName: "test"},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(time.Hour),
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})), cert
}
