package tls

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/filtertest"
)

const (
	signingCA = `Certificate:
    Data:
        Version: 3 (0x2)
        Serial Number: 2 (0x2)
        Signature Algorithm: sha1WithRSAEncryption
        Issuer: DC=org, DC=cheese, O=Cheese, O=Cheese 2, OU=Cheese Section, OU=Cheese Section 2, CN=Simple Root CA, CN=Simple Root CA 2, C=FR, C=US, L=TOULOUSE, L=LYON, ST=Root State, ST=Root State 2/emailAddress=root@signing.com/emailAddress=root2@signing.com
        Validity
            Not Before: Dec  6 11:10:09 2018 GMT
            Not After : Dec  5 11:10:09 2028 GMT
        Subject: DC=org, DC=cheese, O=Cheese, O=Cheese 2, OU=Simple Signing Section, OU=Simple Signing Section 2, CN=Simple Signing CA, CN=Simple Signing CA 2, C=FR, C=US, L=TOULOUSE, L=LYON, ST=Signing State, ST=Signing State 2/emailAddress=simple@signing.com/emailAddress=simple2@signing.com
        Subject Public Key Info:
            Public Key Algorithm: rsaEncryption
                RSA Public-Key: (2048 bit)
                Modulus:
                    00:c3:9d:9f:61:15:57:3f:78:cc:e7:5d:20:e2:3e:
                    2e:79:4a:c3:3a:0c:26:40:18:db:87:08:85:c2:f7:
                    af:87:13:1a:ff:67:8a:b7:2b:58:a7:cc:89:dd:77:
                    ff:5e:27:65:11:80:82:8f:af:a0:af:25:86:ec:a2:
                    4f:20:0e:14:15:16:12:d7:74:5a:c3:99:bd:3b:81:
                    c8:63:6f:fc:90:14:86:d2:39:ee:87:b2:ff:6d:a5:
                    69:da:ab:5a:3a:97:cd:23:37:6a:4b:ba:63:cd:a1:
                    a9:e6:79:aa:37:b8:d1:90:c9:24:b5:e8:70:fc:15:
                    ad:39:97:28:73:47:66:f6:22:79:5a:b0:03:83:8a:
                    f1:ca:ae:8b:50:1e:c8:fa:0d:9f:76:2e:00:c2:0e:
                    75:bc:47:5a:b6:d8:05:ed:5a:bc:6d:50:50:36:6b:
                    ab:ab:69:f6:9b:1b:6c:7e:a8:9f:b2:33:3a:3c:8c:
                    6d:5e:83:ce:17:82:9e:10:51:a6:39:ec:98:4e:50:
                    b7:b1:aa:8b:ac:bb:a1:60:1b:ea:31:3b:b8:0a:ea:
                    63:41:79:b5:ec:ee:19:e9:85:8e:f3:6d:93:80:da:
                    98:58:a2:40:93:a5:53:eb:1d:24:b6:66:07:ec:58:
                    10:63:e7:fa:6e:18:60:74:76:15:39:3c:f4:95:95:
                    7e:df
                Exponent: 65537 (0x10001)
        X509v3 extensions:
            X509v3 Key Usage: critical
                Certificate Sign, CRL Sign
            X509v3 Basic Constraints: critical
                CA:TRUE, pathlen:0
            X509v3 Subject Key Identifier:
                1E:52:A2:E8:54:D5:37:EB:D5:A8:1D:E4:C2:04:1D:37:E2:F7:70:03
            X509v3 Authority Key Identifier:
                keyid:36:70:35:AA:F0:F6:93:B2:86:5D:32:73:F9:41:5A:3F:3B:C8:BC:8B

    Signature Algorithm: sha1WithRSAEncryption
         76:f3:16:21:27:6d:a2:2e:e8:18:49:aa:54:1e:f8:3b:07:fa:
         65:50:d8:1f:a2:cf:64:6c:15:e0:0f:c8:46:b2:d7:b8:0e:cd:
         05:3b:06:fb:dd:c6:2f:01:ae:bd:69:d3:bb:55:47:a9:f6:e5:
         ba:be:4b:45:fb:2e:3c:33:e0:57:d4:3e:8e:3e:11:f2:0a:f1:
         7d:06:ab:04:2e:a5:76:20:c2:db:a4:68:5a:39:00:62:2a:1d:
         c2:12:b1:90:66:8c:36:a8:fd:83:d1:1b:da:23:a7:1d:5b:e6:
         9b:40:c4:78:25:c7:b7:6b:75:35:cf:bb:37:4a:4f:fc:7e:32:
         1f:8c:cf:12:d2:c9:c8:99:d9:4a:55:0a:1e:ac:de:b4:cb:7c:
         bf:c4:fb:60:2c:a8:f7:e7:63:5c:b0:1c:62:af:01:3c:fe:4d:
         3c:0b:18:37:4c:25:fc:d0:b2:f6:b2:f1:c3:f4:0f:53:d6:1e:
         b5:fa:bc:d8:ad:dd:1c:f5:45:9f:af:fe:0a:01:79:92:9a:d8:
         71:db:37:f3:1e:bd:fb:c7:1e:0a:0f:97:2a:61:f3:7b:19:93:
         9c:a6:8a:69:cd:b0:f5:91:02:a5:1b:10:f4:80:5d:42:af:4e:
         82:12:30:3e:d3:a7:11:14:ce:50:91:04:80:d7:2a:03:ef:71:
         10:b8:db:a5
-----BEGIN CERTIFICATE-----
MIIFzTCCBLWgAwIBAgIBAjANBgkqhkiG9w0BAQUFADCCAWQxEzARBgoJkiaJk/Is
ZAEZFgNvcmcxFjAUBgoJkiaJk/IsZAEZFgZjaGVlc2UxDzANBgNVBAoMBkNoZWVz
ZTERMA8GA1UECgwIQ2hlZXNlIDIxFzAVBgNVBAsMDkNoZWVzZSBTZWN0aW9uMRkw
FwYDVQQLDBBDaGVlc2UgU2VjdGlvbiAyMRcwFQYDVQQDDA5TaW1wbGUgUm9vdCBD
QTEZMBcGA1UEAwwQU2ltcGxlIFJvb3QgQ0EgMjELMAkGA1UEBhMCRlIxCzAJBgNV
BAYTAlVTMREwDwYDVQQHDAhUT1VMT1VTRTENMAsGA1UEBwwETFlPTjETMBEGA1UE
CAwKUm9vdCBTdGF0ZTEVMBMGA1UECAwMUm9vdCBTdGF0ZSAyMR8wHQYJKoZIhvcN
AQkBFhByb290QHNpZ25pbmcuY29tMSAwHgYJKoZIhvcNAQkBFhFyb290MkBzaWdu
aW5nLmNvbTAeFw0xODEyMDYxMTEwMDlaFw0yODEyMDUxMTEwMDlaMIIBhDETMBEG
CgmSJomT8ixkARkWA29yZzEWMBQGCgmSJomT8ixkARkWBmNoZWVzZTEPMA0GA1UE
CgwGQ2hlZXNlMREwDwYDVQQKDAhDaGVlc2UgMjEfMB0GA1UECwwWU2ltcGxlIFNp
Z25pbmcgU2VjdGlvbjEhMB8GA1UECwwYU2ltcGxlIFNpZ25pbmcgU2VjdGlvbiAy
MRowGAYDVQQDDBFTaW1wbGUgU2lnbmluZyBDQTEcMBoGA1UEAwwTU2ltcGxlIFNp
Z25pbmcgQ0EgMjELMAkGA1UEBhMCRlIxCzAJBgNVBAYTAlVTMREwDwYDVQQHDAhU
T1VMT1VTRTENMAsGA1UEBwwETFlPTjEWMBQGA1UECAwNU2lnbmluZyBTdGF0ZTEY
MBYGA1UECAwPU2lnbmluZyBTdGF0ZSAyMSEwHwYJKoZIhvcNAQkBFhJzaW1wbGVA
c2lnbmluZy5jb20xIjAgBgkqhkiG9w0BCQEWE3NpbXBsZTJAc2lnbmluZy5jb20w
ggEiMA0GCSqGSIb3DQEBAQUAA4IBDwAwggEKAoIBAQDDnZ9hFVc/eMznXSDiPi55
SsM6DCZAGNuHCIXC96+HExr/Z4q3K1inzIndd/9eJ2URgIKPr6CvJYbsok8gDhQV
FhLXdFrDmb07gchjb/yQFIbSOe6Hsv9tpWnaq1o6l80jN2pLumPNoanmeao3uNGQ
ySS16HD8Fa05lyhzR2b2InlasAODivHKrotQHsj6DZ92LgDCDnW8R1q22AXtWrxt
UFA2a6urafabG2x+qJ+yMzo8jG1eg84Xgp4QUaY57JhOULexqousu6FgG+oxO7gK
6mNBebXs7hnphY7zbZOA2phYokCTpVPrHSS2ZgfsWBBj5/puGGB0dhU5PPSVlX7f
AgMBAAGjZjBkMA4GA1UdDwEB/wQEAwIBBjASBgNVHRMBAf8ECDAGAQH/AgEAMB0G
A1UdDgQWBBQeUqLoVNU369WoHeTCBB034vdwAzAfBgNVHSMEGDAWgBQ2cDWq8PaT
soZdMnP5QVo/O8i8izANBgkqhkiG9w0BAQUFAAOCAQEAdvMWISdtoi7oGEmqVB74
Owf6ZVDYH6LPZGwV4A/IRrLXuA7NBTsG+93GLwGuvWnTu1VHqfblur5LRfsuPDPg
V9Q+jj4R8grxfQarBC6ldiDC26RoWjkAYiodwhKxkGaMNqj9g9Eb2iOnHVvmm0DE
eCXHt2t1Nc+7N0pP/H4yH4zPEtLJyJnZSlUKHqzetMt8v8T7YCyo9+djXLAcYq8B
PP5NPAsYN0wl/NCy9rLxw/QPU9Yetfq82K3dHPVFn6/+CgF5kprYcds38x69+8ce
Cg+XKmHzexmTnKaKac2w9ZECpRsQ9IBdQq9OghIwPtOnERTOUJEEgNcqA+9xELjb
pQ==
-----END CERTIFICATE-----
`

	minimalCheeseCrt = `-----BEGIN CERTIFICATE-----
MIIEQDCCAygCFFRY0OBk/L5Se0IZRj3CMljawL2UMA0GCSqGSIb3DQEBCwUAMIIB
hDETMBEGCgmSJomT8ixkARkWA29yZzEWMBQGCgmSJomT8ixkARkWBmNoZWVzZTEP
MA0GA1UECgwGQ2hlZXNlMREwDwYDVQQKDAhDaGVlc2UgMjEfMB0GA1UECwwWU2lt
cGxlIFNpZ25pbmcgU2VjdGlvbjEhMB8GA1UECwwYU2ltcGxlIFNpZ25pbmcgU2Vj
dGlvbiAyMRowGAYDVQQDDBFTaW1wbGUgU2lnbmluZyBDQTEcMBoGA1UEAwwTU2lt
cGxlIFNpZ25pbmcgQ0EgMjELMAkGA1UEBhMCRlIxCzAJBgNVBAYTAlVTMREwDwYD
VQQHDAhUT1VMT1VTRTENMAsGA1UEBwwETFlPTjEWMBQGA1UECAwNU2lnbmluZyBT
dGF0ZTEYMBYGA1UECAwPU2lnbmluZyBTdGF0ZSAyMSEwHwYJKoZIhvcNAQkBFhJz
aW1wbGVAc2lnbmluZy5jb20xIjAgBgkqhkiG9w0BCQEWE3NpbXBsZTJAc2lnbmlu
Zy5jb20wHhcNMTgxMjA2MTExMDM2WhcNMjEwOTI1MTExMDM2WjAzMQswCQYDVQQG
EwJGUjETMBEGA1UECAwKU29tZS1TdGF0ZTEPMA0GA1UECgwGQ2hlZXNlMIIBIjAN
BgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAskX/bUtwFo1gF2BTPNaNcTUMaRFu
FMZozK8IgLjccZ4kZ0R9oFO6Yp8Zl/IvPaf7tE26PI7XP7eHriUdhnQzX7iioDd0
RZa68waIhAGc+xPzRFrP3b3yj3S2a9Rve3c0K+SCV+EtKAwsxMqQDhoo9PcBfo5B
RHfht07uD5MncUcGirwN+/pxHV5xzAGPcc7On0/5L7bq/G+63nhu78zw9XyuLaHC
PM5VbOUvpyIESJHbMMzTdFGL8ob9VKO+Kr1kVGdEA9i8FLGl3xz/GBKuW/JD0xyW
DrU29mri5vYWHmkuv7ZWHGXnpXjTtPHwveE9/0/ArnmpMyR9JtqFr1oEvQIDAQAB
MA0GCSqGSIb3DQEBCwUAA4IBAQBHta+NWXI08UHeOkGzOTGRiWXsOH2dqdX6gTe9
xF1AIjyoQ0gvpoGVvlnChSzmlUj+vnx/nOYGIt1poE3hZA3ZHZD/awsvGyp3GwWD
IfXrEViSCIyF+8tNNKYyUcEO3xdAsAUGgfUwwF/mZ6MBV5+A/ZEEILlTq8zFt9dV
vdKzIt7fZYxYBBHFSarl1x8pDgWXlf3hAufevGJXip9xGYmznF0T5cq1RbWJ4be3
/9K7yuWhuBYC3sbTbCneHBa91M82za+PIISc1ygCYtWSBoZKSAqLk0rkZpHaekDP
WqeUSNGYV//RunTeuRDAf5OxehERb1srzBXhRZ3cZdzXbgR/
-----END CERTIFICATE-----
`

	minimalCert = `-----BEGIN CERTIFICATE-----
MIIDGTCCAgECCQCqLd75YLi2kDANBgkqhkiG9w0BAQsFADBYMQswCQYDVQQGEwJG
UjETMBEGA1UECAwKU29tZS1TdGF0ZTERMA8GA1UEBwwIVG91bG91c2UxITAfBgNV
BAoMGEludGVybmV0IFdpZGdpdHMgUHR5IEx0ZDAeFw0xODA3MTgwODI4MTZaFw0x
ODA4MTcwODI4MTZaMEUxCzAJBgNVBAYTAkZSMRMwEQYDVQQIDApTb21lLVN0YXRl
MSEwHwYDVQQKDBhJbnRlcm5ldCBXaWRnaXRzIFB0eSBMdGQwggEiMA0GCSqGSIb3
DQEBAQUAA4IBDwAwggEKAoIBAQC/+frDMMTLQyXG34F68BPhQq0kzK4LIq9Y0/gl
FjySZNn1C0QDWA1ubVCAcA6yY204I9cxcQDPNrhC7JlS5QA8Y5rhIBrqQlzZizAi
Rj3NTrRjtGUtOScnHuJaWjLy03DWD+aMwb7q718xt5SEABmmUvLwQK+EjW2MeDwj
y8/UEIpvrRDmdhGaqv7IFpIDkcIF7FowJ/hwDvx3PMc+z/JWK0ovzpvgbx69AVbw
ZxCimeha65rOqVi+lEetD26le+WnOdYsdJ2IkmpPNTXGdfb15xuAc+gFXfMCh7Iw
3Ynl6dZtZM/Ok2kiA7/OsmVnRKkWrtBfGYkI9HcNGb3zrk6nAgMBAAEwDQYJKoZI
hvcNAQELBQADggEBAC/R+Yvhh1VUhcbK49olWsk/JKqfS3VIDQYZg1Eo+JCPbwgS
I1BSYVfMcGzuJTX6ua3m/AHzGF3Tap4GhF4tX12jeIx4R4utnjj7/YKkTvuEM2f4
xT56YqI7zalGScIB0iMeyNz1QcimRl+M/49au8ow9hNX8C2tcA2cwd/9OIj/6T8q
SBRHc6ojvbqZSJCO0jziGDT1L3D+EDgTjED4nd77v/NRdP+egb0q3P0s4dnQ/5AV
aQlQADUn61j3ScbGJ4NSeZFFvsl38jeRi/MEzp0bGgNBcPj6JHi7qbbauZcZfQ05
jECvgAY7Nfd9mZ1KtyNaW31is+kag7NsvjxU/kM=
-----END CERTIFICATE-----`
)

func TestPassTLSClientCert_PEM(t *testing.T) {
	testCases := []struct {
		desc           string
		certContents   []string // set the request TLS attribute if defined
		expectedHeader string
	}{
		{
			desc: "No TLS",
		},
		{
			desc:           "TLS with simple certificate",
			certContents:   []string{minimalCheeseCrt},
			expectedHeader: getCleanCertContents([]string{minimalCert}),
		},
		{
			desc:           "TLS with complete certificate",
			certContents:   []string{minimalCheeseCrt},
			expectedHeader: getCleanCertContents([]string{minimalCheeseCrt}),
		},
		{
			desc:           "TLS with two certificates",
			certContents:   []string{minimalCert, minimalCheeseCrt},
			expectedHeader: getCleanCertContents([]string{minimalCert, minimalCheeseCrt}),
		},
	}

	for _, test := range testCases {
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			spec := New()
			assert.Equal(t, spec.Name(), filters.TLSName)

			f, err := spec.CreateFilter([]any{})
			require.NoError(t, err)

			req, err := http.NewRequest(http.MethodGet, "http://example.com/foo", nil)
			require.NoError(t, err)

			if len(test.certContents) > 0 {
				req.TLS = buildTLSWith(test.certContents)
			}

			ctx := &filtertest.Context{
				FRequest: req,
			}
			f.Request(ctx)

			if test.expectedHeader != "" {
				expected := getCleanCertContents(test.certContents)
				assert.Equal(t, expected, req.Header.Get(certHeaderName), "The request header should contain the cleaned certificate")
			} else {
				assert.Empty(t, req.Header.Get(certHeaderName))
			}

		})
	}
}

func Test_sanitize(t *testing.T) {
	testCases := []struct {
		desc       string
		toSanitize []byte
		expected   string
	}{
		{
			desc: "Empty",
		},
		{
			desc:       "With a minimal cert",
			toSanitize: []byte(minimalCheeseCrt),
			expected: `MIIEQDCCAygCFFRY0OBk/L5Se0IZRj3CMljawL2UMA0GCSqGSIb3DQEBCwUAMIIB
hDETMBEGCgmSJomT8ixkARkWA29yZzEWMBQGCgmSJomT8ixkARkWBmNoZWVzZTEP
MA0GA1UECgwGQ2hlZXNlMREwDwYDVQQKDAhDaGVlc2UgMjEfMB0GA1UECwwWU2lt
cGxlIFNpZ25pbmcgU2VjdGlvbjEhMB8GA1UECwwYU2ltcGxlIFNpZ25pbmcgU2Vj
dGlvbiAyMRowGAYDVQQDDBFTaW1wbGUgU2lnbmluZyBDQTEcMBoGA1UEAwwTU2lt
cGxlIFNpZ25pbmcgQ0EgMjELMAkGA1UEBhMCRlIxCzAJBgNVBAYTAlVTMREwDwYD
VQQHDAhUT1VMT1VTRTENMAsGA1UEBwwETFlPTjEWMBQGA1UECAwNU2lnbmluZyBT
dGF0ZTEYMBYGA1UECAwPU2lnbmluZyBTdGF0ZSAyMSEwHwYJKoZIhvcNAQkBFhJz
aW1wbGVAc2lnbmluZy5jb20xIjAgBgkqhkiG9w0BCQEWE3NpbXBsZTJAc2lnbmlu
Zy5jb20wHhcNMTgxMjA2MTExMDM2WhcNMjEwOTI1MTExMDM2WjAzMQswCQYDVQQG
EwJGUjETMBEGA1UECAwKU29tZS1TdGF0ZTEPMA0GA1UECgwGQ2hlZXNlMIIBIjAN
BgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAskX/bUtwFo1gF2BTPNaNcTUMaRFu
FMZozK8IgLjccZ4kZ0R9oFO6Yp8Zl/IvPaf7tE26PI7XP7eHriUdhnQzX7iioDd0
RZa68waIhAGc+xPzRFrP3b3yj3S2a9Rve3c0K+SCV+EtKAwsxMqQDhoo9PcBfo5B
RHfht07uD5MncUcGirwN+/pxHV5xzAGPcc7On0/5L7bq/G+63nhu78zw9XyuLaHC
PM5VbOUvpyIESJHbMMzTdFGL8ob9VKO+Kr1kVGdEA9i8FLGl3xz/GBKuW/JD0xyW
DrU29mri5vYWHmkuv7ZWHGXnpXjTtPHwveE9/0/ArnmpMyR9JtqFr1oEvQIDAQAB
MA0GCSqGSIb3DQEBCwUAA4IBAQBHta+NWXI08UHeOkGzOTGRiWXsOH2dqdX6gTe9
xF1AIjyoQ0gvpoGVvlnChSzmlUj+vnx/nOYGIt1poE3hZA3ZHZD/awsvGyp3GwWD
IfXrEViSCIyF+8tNNKYyUcEO3xdAsAUGgfUwwF/mZ6MBV5+A/ZEEILlTq8zFt9dV
vdKzIt7fZYxYBBHFSarl1x8pDgWXlf3hAufevGJXip9xGYmznF0T5cq1RbWJ4be3
/9K7yuWhuBYC3sbTbCneHBa91M82za+PIISc1ygCYtWSBoZKSAqLk0rkZpHaekDP
WqeUSNGYV//RunTeuRDAf5OxehERb1srzBXhRZ3cZdzXbgR/`,
		},
	}

	for _, test := range testCases {
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			content := sanitize(test.toSanitize)

			expected := strings.ReplaceAll(test.expected, "\n", "")
			assert.Equal(t, expected, content, "The sanitized certificates should be equal")
		})
	}
}

func getCleanCertContents(certContents []string) string {
	exp := regexp.MustCompile("-----BEGIN CERTIFICATE-----(?s)(.*)")

	var cleanedCertContent []string
	for _, certContent := range certContents {
		cert := sanitize([]byte(exp.FindString(certContent)))
		cleanedCertContent = append(cleanedCertContent, cert)
	}

	return strings.Join(cleanedCertContent, certSeparator)
}

func buildTLSWith(certContents []string) *tls.ConnectionState {
	var peerCertificates []*x509.Certificate

	for _, certContent := range certContents {
		peerCertificates = append(peerCertificates, getCertificate(certContent))
	}

	return &tls.ConnectionState{PeerCertificates: peerCertificates}
}

func getCertificate(certContent string) *x509.Certificate {
	roots := x509.NewCertPool()
	ok := roots.AppendCertsFromPEM([]byte(signingCA))
	if !ok {
		panic("failed to parse root certificate")
	}

	block, _ := pem.Decode([]byte(certContent))
	if block == nil {
		panic("failed to parse certificate PEM")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		panic("failed to parse certificate: " + err.Error())
	}

	return cert
}
