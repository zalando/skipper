package main

import (
	"bytes"
	"compress/zlib"
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/x509"
	_ "embed"
	"encoding/asn1"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"

	"github.com/fxamacker/cbor/v2"
)

//go:embed Apple_App_Attestation_Root_CA.pem
var appleRootCertBytes []byte

// This comes from the device and is sent as the header - it is ZLIB encoded
var encodedAttestation = []byte(`7VgJVFPHGk5uLmELmyggolyXUnbmJoQk8FQ2FYyIsihgRS_JTQjNgsllc6kkWhSOWNcK1loURHGjdcG6AK-1vKqotYp9ikVLFbRVammxrpQ3lwAGn93O6Xk9553mnLmTf-7_z0zm_77__yflEpmK0hCZmUrSDz4JiiJ1lBx28ZSK2i7J5UsMych9YEDuwqYtYyFMBGEzC_dsO7QeWLEtvAuizo9HWQiIxceC0WxWIsriuIbRs2HwiYX1TkdQCo0aiwjDcNwRONBKVhwro1K0WuLfP2jBsYoglAqZRqtWEGCUsw2XBwQ8gONAwBWm9IpCwO0TgX4tHg2mGFcMlUmBhExLE-FAKgQkIAipUAiCgEBA8NJkOBkoEEi5Mi6XHyjCubwgKSECokBpIC4hSVwaCODTFbjQU1lzHMLCwrAIUkspZApJ78b_wJ6TgSPbvPdIEGbf2bCY5qxwBpr1Se6eKVN7pq2YpfkECa0s0T6s2jAsprCw-W64e7W4kxyz6k3Rg0ZfLs9pVtVNzWeu9_Z6KgMqQ4a9PWqaY3fw1Ihy_U9A_yPgwDVHOjKZPSgCGMCWluxoCXoA_R7o89mWcNGChxKphRmqAwsqoLus6gsB7Jj1hTjsGPWFXKPEM3aBnqgHPyZudpSQx5s6yV-iUfmrshaqCEqS_vwLocxMJ3awUVT3ug6rLwyCpmb1hQLjfCJjF0x3YMTzHZijtoBTX5RrgbLxQP9Afy7gPX-JoB5g3LYxKBaxJlurtXvz-MGbRSuqWKJdVz00LTsXbUr5rGxn1fIJX20yBRlLzgBSBDjNX121dV11Wve9fbXXRC1fzk3S1o4fsTXgyISnymr_j3ZyrK_v31p7wWPn2oAoQ76sCwHUBMXZXakHjvh80K6rwkvz2qpWVS22We6xSOjGzImRzStcnsTpcTl6cnfziAuPSvzqk5EpEO0RwMA8ZUS8veWxhtYToa5Xm-I3nhC6WiSYbIsF4nD4Y4xIHPly7MdpNBQkwB-AEg1_AHi4EBfyRHw-hD-PFuGD_qT8-XzLBvbPsWvmo0cZY1hpDHT_HEFZTkzeAf5bobc__8HDo_Pjefed8_2-PWjISF4R--jivmGv5G7eLkOHD4mq-cdROXtrySVJbarArr5sNXdrvq7qzduUujx5-08ar2vc6kmpo52SLsurt3q8cYCI-dhVdr60XAakYMgArC0AG3Y0ktzpsbGoC3DKH7p3rX08r7YuPaxnJ6eq7cm3bW1F28BIWsEWdUKHTrg51w0dXnX6EP5oOfVFa-ts7c13BpODyTZ1GEvBADIEZxyvW1msKzZDPjvRLVNZzp7BP_HFCt-Jy0G0h1fH54-H3skrddt85_D-R5d2JfAe1tiwockeYceFs_oORkXcyWORj9dLyPifz3ot9T90473UupSpZ2pKXxMbbB6jQk2u-fGkDaIjci0pIRWZVLJtGuhnpw3THCnLB_mQf7gdsGFbzi-IYpIsFJKKYarELMsfl48aWHdwAzoK-NM4ZDJ_L1shfh8jTJrjqOHv-P079_x3_P7_iN_AE2GiNPSx1HEbU18vrSghJ3x53Td3YdyNdxd_f2FpRIqw8Efn8IjJz8B8eBS0ZtLkoOypkxSzghIjJ_nIpRk-SkkOSMTTJoXlRC0QiiMyxAtiMiLyMsRcjZoksxcKfARScQxQZoRJ1dqcxBm8gKDpghnZ2eLAIFWANCp9pjY8KU0mHz8e2EIs0SuwwxISJsUnADsE8h_K5jpCLU3T5AIMYXLoARc0mAu4PD8g8OPhCTgeDATBXKG_QCRIoXWG9er0auDAjysarMHo_cCwYmDtgy2hL28tOVc602_VknNO1tdthfqF4YM4vxgHwN_IwFcHOK_sow2kCUXKtQPc52N-2BS8P9NZ92e6QVTDwrKodI1WQeW9nHTWwJIeZDsiifF9AQLGBJzPFeECOkAEgkAQ1CsGpYAUPAgEGnfnZ7ov05g0WUtkSbE4Y4zF4hVytUIt_x1r_zLRm0s85ffnXvTaO6K2bssCy46J_xbvGJ7n8n6k0pUb0D5rdvS9xx8cv4zqEGX7hisjL52yaDG8Mepg49PObm-vr5fad5cbmFdg3dD0AtVNs9lV55_F8jVC8bqua-cvz84fav3W-NMggm3hw2aamZlDJwsAH3KtTwbMglfTKSozOCBAI9Fl-veW6jTPe0XA8yMIeDJ8OQ5wuK4bvQ6GGpiOULCDzXqAs2ZMoO8G-pP9EyMI0Ndw9AfjSKWCUEtIDJ4nla7QYZIBl5JYWh5GqPOwTEJLwW86XZaK1GGEREJmUkYbGbQh6abGCKOT0uCJQ_-opYRWilGkVgUN1FJMolFLFbTTdLRRlo70HbRQpgba5hk1B0EqU0tIKIWkd06KVJFqSucP-AM_gunk1Xc6OTk5JodjMjfRD0qTkmFa58737IkiV5H0rKr9Woh52qbs2sHx2Dwf2A2cHccORWByNo1lUxggEsEWXpN-nbUl1sypxzf62o9rfAMcfCtzVs8wWFHLtuwNeecegk3ifzrfmxsa89hbL3-2qUb7lWVpk1fR27fNGjssM51nFNOJGralfYSd9XRZk09P8UlB6djDKfvVtYMKTTk-Agw3UmKIEdp9hSXNTd6fy01cBHhcLi7g83g8uvoMpMX-6vOvjRy_zN515yW1Swo-2ONpFeLWWv0U3ZC_du9P-I3KRk--JlLcOrptdsuQ4uJjhsUb66Kf1PHO7D-HDu8MiHjlSVfdwcZlbxwr1z8E-i7o-H7umgEW7AbR9_j717cl8VZurigqvV5Xe7e6uXHcHjDZhL7BsMwJMqGv96_Tlx7QQjdKCDmsl-iF3FF4usC3zLvMs8Cjz1iiVZrYDjLyh-9MoP3S8PJiNWzPtuqDNhthvYBtFiudAUhY7BZrKt_b9tWte3Om64rWLR2fG1_yVleze9Kok1lTfxbumua9zNV1t02lVNV6dP2dzxOndSBAHuJ7qPWOwDyn8fK97F2uwYkhE_kZpxra173fStTlRytuzfOpUrY6ILvPEleFR3RT-25Zp43gt_BreLbyYoN4418HepiBYMUqhOAPokEvMhH_dxv5hatYyTcB40O1FR3rx2j1jm4t9hs7G8qnL_hyqNNV27ZKM-8c6aJ504WXjrtFHl76wL2Gmzb39DfL58SCWIcURndSEmfHAzvfGPMpjlnNmzZt0uCvdfi8uwgLuXI04WpqaUXwTCu8PByEmuDopTj_L2b82jWrD0bLvvnoE6dU17mBLlc77ncpbRmTxR_eODDZocGuu33hqm2Vr5__mNW05cH6f1EfZtzYPXYYAlSyIvszTTZNseeKIyYcKpLs4Klvqpqdj0jdT8V7PF6ZRomsnec0ztzFuePCur39gCeDgeufAT288MAct-avjU4vqbleuOKZZBF0Mp1EmDGK9sMtXZ2WT-yja1RjHWKDBe3TX-1Jf7t-KMoh9rsjWB765Gy3m3zf082ycNfb4Te_G3WkQXzjn_HZh7T6H9jRUcbqL53OdJEERSRV1JwpOLdu3pM1MbZ7vl9dtGpJ8kNh_TveLbcw-aftGW1d3aG0PtH_f5-UzCaVmkwG1s25e9T-iq3VvpVs84UhnbdyLv7wSvSQoOY1G5rcvhsZ-t0OJsLywJijk7DfvBAlYb91J_oP`)

// Challenge that was issued to the client
var encodedChallengeData = []byte(`NzY4RTBGQTQtQkI0MC00NjA3LUI3QjktRTBBRTcyODBCQ0U3`)

func getAttestationCbor() *attestationCbor {
	var decodedAttestationPayload = make([]byte, base64.URLEncoding.DecodedLen(len(encodedAttestation)))
	if _, decodeErr := base64.URLEncoding.Decode(decodedAttestationPayload, encodedAttestation); decodeErr != nil {
		slog.Error("cannot decode payload", "error", decodeErr)
		return nil
	}

	// iOS implements level 5 zlib compression - https://developer.apple.com/documentation/compression/compression_zlib
	// iOS doesn't prepend the zlib magic bytes so we're adding them, found via https://stackoverflow.com/a/43170354
	decodedAttestationPayload = append([]byte{0x78, 0x5E}, decodedAttestationPayload...) // iOS doesn't append the

	reader, readerErr := zlib.NewReader(bytes.NewBuffer(decodedAttestationPayload))
	if readerErr != nil {
		slog.Error("cannot create reader", "error", readerErr)
		return nil
	}

	var attestationCborData bytes.Buffer
	_, _ = io.Copy(&attestationCborData, reader)
	_ = reader.Close()

	fmt.Println(hex.EncodeToString(attestationCborData.Bytes()))

	var attestation attestationCbor
	if err := cbor.Unmarshal(attestationCborData.Bytes(), &attestation); err != nil {
		slog.Error("cannot read CBOR data", "error", err)
	}
	return &attestation
}

func main() {
	attestation := getAttestationCbor()

	// The steps below come from https://developer.apple.com/documentation/devicecheck/validating_apps_that_connect_to_your_server
	// See the section "Verify the Attestation"

	// Alex notes: where I got stuck on this is the iOS client needs to send some authenticator data to the server in
	// addition to the attestation object

	// This post looks useful - https://blog.restlesslabs.com/john/ios-app-attest (see the Python psuedocode)
	// There might also be something to do this too - https://github.com/takimoto3/app-attest

	// Step 1.
	// Verify that the x5c array contains the intermediate and leaf certificates for App Attest,
	// starting from the credential certificate in the first data buffer in the array (credcert).
	// Verify the validity of the certificates using Apple’s App Attest root certificate.
	var credCert *x509.Certificate
	{
		if attestation.Fmt != "apple-appattest" {
			slog.Error("fmt is not apple-appattest")
			return
		}

		// If x5c is not present, return an error
		if len(attestation.AttStmt.X5C) == 0 {
			slog.Error("x5c is not present")
			return
		}

		if len(attestation.AttStmt.X5C) != 2 {
			slog.Error("x5c is not of length 2")
			return
		}

		// Step 2. Verify the validity of Apple's certificate chain
		_credCert, parseLeafCertErr := x509.ParseCertificate(attestation.AttStmt.X5C[0])
		if parseLeafCertErr != nil {
			slog.Error("failed to parse leaf certificate", "error", parseLeafCertErr)
			return
		}

		intermediateCert, parseIntermediateCertErr := x509.ParseCertificate(attestation.AttStmt.X5C[1])
		if parseIntermediateCertErr != nil {
			slog.Error("failed to parse intermediate certificate", "error", parseIntermediateCertErr)
			return
		}
		intermediaryPool := x509.NewCertPool()
		intermediaryPool.AddCert(intermediateCert)

		rootPool := x509.NewCertPool()
		ok := rootPool.AppendCertsFromPEM(appleRootCertBytes)
		if !ok {
			slog.Error("failed to create root certificate pool")
			return
		}

		_, verifyErr := _credCert.Verify(
			x509.VerifyOptions{
				DNSName:       "", // Should be empty
				Intermediates: intermediaryPool,
				Roots:         rootPool,
			},
		)
		if verifyErr != nil {
			slog.Error("Unable to verify certificate", "error", verifyErr)
		}
		credCert = _credCert
	}

	// Step 2.
	// Create clientDataHash as the SHA256 hash of the one-time challenge your server sends to your app before
	// performing the attestation, and append that hash to the end of the authenticator data
	// (authData from the decoded object).
	var clientDataHash []byte
	{
		clientDataHasher := sha256.New()
		clientDataHasher.Write(encodedChallengeData)
		clientDataHash = clientDataHasher.Sum(nil)
	}

	// Step 3.
	// Generate a new SHA256 hash of the composite item to create nonce.
	var nonce []byte
	{
		nonceHasher := sha256.New()
		nonceHasher.Write(attestation.AuthData)
		nonceHasher.Write(clientDataHash)
		nonce = nonceHasher.Sum(nil)
	}

	// Step 4.
	// Obtain the value of the credCert extension with OID 1.2.840.113635.100.8.2, which is a DER-encoded ASN.1
	// sequence. Decode the sequence and extract the single octet string that it contains.
	// Verify that the string equals nonce.
	{
		var attExtBytes []byte
		for _, ext := range credCert.Extensions {
			if ext.Id.Equal([]int{1, 2, 840, 113635, 100, 8, 2}) {
				attExtBytes = ext.Value
			}
		}
		if len(attExtBytes) == 0 {
			slog.Error("Attestation certificate extensions missing 1.2.840.113635.100.8.2")
		}
		decoded := AppleAnonymousAttestation{}
		if _, err := asn1.Unmarshal(attExtBytes, &decoded); err != nil {
			slog.Error("Unable to parse apple attestation certificate extensions")
		}

		if !bytes.Equal(decoded.Nonce, nonce) {
			slog.Error("Attestation certificate does not contain expected nonce")
		}
	}

	// Step 5.
	// Create the SHA256 hash of the public key in credCert, and verify that it matches the key identifier from
	// your app.
	var publicKey *ecdsa.PublicKey
	{
		_pubKey := credCert.PublicKey.(*ecdsa.PublicKey)
		// nonceHasher.Write(elliptic.Marshal(pubKey, pubKey.X, pubKey.Y))
		// certSha256 := fmt.Sprintf("%x", nonceHasher.Sum(nil))
		// fmt.Println(certSha256)
		publicKey = _pubKey
	}

	// Step 6.
	// Compute the SHA256 hash of your app’s App ID, and verify that it’s the same as the authenticator
	// data’s RP ID hash.
	{
		// TODO where to get the authenticator data from?
	}

	// Step 7.
	// Verify that the authenticator data’s counter field equals 0.
	{
		// TODO where to get the authenticator data from?
	}

	// Step 8.
	// Verify that the authenticator data’s aaguid field is either appattestdevelop if operating in the
	// development environment, or appattest followed by seven 0x00 bytes if operating in the production environment.
	{
		// TODO where to get the authenticator data from?
	}

	// Step 9.
	// Verify that the authenticator data’s credentialId field is the same as the key identifier.
	{
		// TODO where to get the authenticator data from?
	}
}

type attestationCbor struct {
	Fmt      string           `cbor:"fmt"`
	AttStmt  appleCborAttStmt `cbor:"attStmt"`
	AuthData []byte           `cbor:"authData"`
}

type appleCborAttStmt struct {
	X5C     [][]byte `cbor:"x5c"`
	Receipt []byte   `cbor:"receipt"`
}

type appleAuthData struct {
	RPIDHash []byte `cbor:"rpid"`
	// Flags    AuthenticatorFlags     `json:"flags"`
	// Counter  uint32                 `json:"sign_count"`
	// AttData  AttestedCredentialData `json:"att_data"`
	// ExtData  []byte                 `json:"ext_data"`
}

// Apple has not yet publish schema for the extension(as of JULY 2021.)
type AppleAnonymousAttestation struct {
	Nonce []byte `asn1:"tag:1,explicit"`
}

// type appleCborCert struct {
// CredCert []byte `cbor:"credCert"`
// CACert   []byte `cbor:"caCert"`
// }
