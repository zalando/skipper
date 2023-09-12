package main

import (
	"bytes"
	"compress/zlib"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/x509"
	_ "embed"
	"encoding/asn1"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"log/slog"

	"github.com/fxamacker/cbor/v2"
)

//go:embed Apple_App_Attestation_Root_CA.pem
var appleRootCertBytes []byte

// This comes from the device and is sent as the header - it is ZLIB encoded
var encodedAttestation = []byte(`7VgJVBPXGk4mQ9gioCgIbuNSZA13MgkkUJUUERARZJHFCg6ThWg2k0HE1kqionBKa12qtLaCqBW1Yl1oFYFXLc_dVsXW5SGlKmIfVG2pipb67hDA4LPbOT2v57zTnNy5-W_-u-T_v-___5tySqGhdaRer5YHwCdJ03IjrYRdIq2hN1MLRZQ5DbkLzEgbbIZSDsJGEC67aFmZ4xngwLXzLYg6OwHlICAOHwtGcznJKIfnKWVWw-ATk3YvR9IqnRYLl2I47goGMkoOPAeLUrSW4vcO2vEcwkm1SqEzaFUkGOk-QEAACQ5woUAowNN7RKJHBKa38GgQadkxTCTHASGWibIkQUEyhSALEIRcIQwWkgQlIBWyYFwmFokUQABwMS4DQTI5FQRIiSA4SyAAgAQE7gmGMks58gZKpVIsXG6gVQoV1X3wP3DmNODKte02CcLusQ2Hbct5iYU2T2xK8Dzgduj7iyF3P508-7x_0d4Hre7ml7FIdFDllyk-FfPrc2JO-r5dUI3dKBE06zcuT-VNq-AFs0Js53zB2_Sw3HQfmH4APLjnCFc2-wmKABZwYiRnRoIeQO8CUz7XHm5a8ICS2dmgRjB_C3SXQ10hgB27rhCHHauuUGCRCEsn9Ea9RLEJKVFigpgawad0Gr4mZ5GGpKnspx9ItT6b3MpFUeM8I1ZXGASn2tQVBlvWk1i6EKYDw56ewBZ1Ary6ooV2KBcX8oV8ASCefomgXmBc2RgUi5hkOt3pTJaU2R-b7zbyeGROqFtWwXrR7PU3qJkjR4_63BpknGwWkCMgu6Ux7nHGuMwE9wfNidKuCNS94thB6sFjn8Ilc_elV7MXv9eYGN-xsnCFKJnbkl9LITir9R9RL34TUKzSNlz38Ll21n_xYdcM9fBYm-avZ611OFZJfFcqzvJwkPK_Ozokk7djpnMaEgnhHg7M7OMWyLvYH6pvPhzmebkhcd1hsaddktW5OCABh7_GAsURzwd_gk5HQwb8ASwx-AcQmWJcTEhEIoh_ghHhg3ml__mEWwBcnoLXxs-EssZwsljo7lnBpbmxeXtFb4bdOve9l9e9I5l33PMDvt1nnpu2Iu7h-Q-HvLDwnc0K1GNQVNWLB5Xc9zdcoGoygp3rSt8QvJ9vrFh-i9aWp22-r_O5KqiMyBjtlnpRWfm-12t7ydgjnoqzJeUKIAOD-nBtB7iwY6A0ihkbiw4FbvmDd73lkkjU1GZLn3zAq7j56NubN4vKwAhGwQl1QwdPvD57OOpRcWI__nAZ_WVzc4rh-rv92cHmWjuMo2IBBYRFde3KYmOxDfL54S6Fxj4lXnT4yxX-k5aBaC-f9nOdg1vzSoa_03pg98ML25OIB1UDuHDKTnH7F6dN7awtCUcPTe5cQ8kTfz7ts4S_v2lTRm361JNVJS_HmAd0omLdQtvq1LWSj5UGOSVX6ek0pyzQS88BbFukNB_kQwLizmAA135OQRRbzkEhq1jWSuzS_HH5qJnTipvREYDP4JDN_r10hfh9hLAZkqPmvwP47zzz3wH8_ySAA2-EjTLYx1bwp8efLVl1aY-U4EWeaDw3rG1efMht8dySK8uX3a4e9BmYA23BaKbGRk2VSYSGZKFSlTYlUBmdNyOKjAkXRsULDMoF8mRNnEoar5YvSs2ZlzgzWxsjnqbG4_OCg7SSmdGKvFxp4NxwuVChShQqQUR2Skx8unJ6oF_kjAkTgBMEE7MDV5qUFJGYBJwRGACgbGsktbIs3UIwCmHzmAFU4i4AAiIASAJwPAkXhgjgG-cLJemMyhBGxaKACwJw0E-B1f2CUcXM-RC2pJ60tfhMyYyA1xefcXO85iQ2LXqpH-VfxQHgWwg4vo_y6h7WQJbQcqWhj_oiLACLxHsTnWNvouvHNEyaQ2frDCo67_mccwT2zCDXFUlO7IkPBBDjIoEED2bigxAIQVC3GJQO0vEgILScLsD6XNYhaYqBzJFhCZYQiyWqlFqVVvk79v5lnl_Z4K28M_u8z65hNbUb59u3T_oqZqtH3tA9k9WegsCWmSnRbZ0fVV9EjYi6Ze2lEReO2zWaXxu579Tje12-Pt8scekqN7MvwbKh4RmmWyezy-4_xyhXiWNWd1w9ezElf7DjmxNOgHCunR-XbWNjC30cDESQaj0yYBeMz6ZpfUhgoI4y6vndpTpD824REAEkCS0jUsI4C_cdzuyDoWa2KxScYXPso6wNG5i6gOlo78IIAkxVPNO-BLlaRWopOQbtSWerjBjV51I5lpWHkdo8TE8aaPjJaMzRyI0YSVFyPW2Zo4Bz5EzTYqTFSVnQ4tA_WhlpkGG03KCBE7QyjNJpZSrGaUZmUo5R7t9vI70Ozs2zaPaDlN5AUrSK6l6TlmvkWtrIB6K-H8F28-mxTm5urpVxrNYme0FpVTFMu_fBJheyyFMiO61puRpqm7V-QU3_cGybD5z7bMdzRhGYm61DWSQLTEawRVdl3-RsjLNxe-IfffWHVf6BA_235b4Rb3agl27cFfpuG4JFiI7N8RWExXb6mpQ_ra8yfG1f0uBT9PYtm1Pt9nr3-GKYlTthW9JD2JmPlzb4PSk-Glwy9kD6bm1NvzpTiQ8DHhZKDLJAu6euZLhJ_LncxCWAEAjwYBFBEEzxKWTE3uLzr40cv8ze1WepmsUFH-30dggd3lz5GF2b_9au-3jTtlPeIt3kmObRN1MaBxUXHzK_uq42-lEtcXL3GdTjXmD4C486avedWvraoXLTA2DqgI7v5a4N4MCuH32r91wrSyVWvrOlqORabc2_K6-cGrcTTLGibwgQgyAr-vr-On2ZAQN0I0UqCRDMbDQKhdYF_qW-pd4FXj2TKYPaam6_SXz4nRW0nxteni2GXbgOPdDmIpxnsM2xpGmcVazbtqns6xtts6Ybi1YvmbAwccObHVdGpY48mjP1Z_H2ab5LPT13DNgm0zQfXNN6LnlaOwKUof77m1uDbXNPXWxbsN0zJDl0kmju8fqW1Xuaydr8aNWNTL8KdfNAZMdp8rL4Y-PUnkvWCQv47QLqf1p5vj5m3V8HepiBCKbGlIAgBvQSK_F_d5BfuIltuB04IcywpX3NGIPJdXijy7p79eXT5_9rsNtlp5vbbHxzZa9kThdfqB4--cCSH0dVCbJmn7i9bFYciBuYzupKTeVt_dHZP9Y20jXnyvr163X4y-1-772ChV46mHQ5o2RLyAwHvPwlEGaFo-fi_L-Y8Wu3rB4YLb396WduGZ6zhUMvt9_pUDuxpsR80rR3ysB6566WRa-XbZt39ginYeOPa_5JfzK3acfYIQjQKIpcTjYMaIg7Uxw-cX8RtZXQXtdccf9YNup4olfnyixa4ug-69SM7bzWoZxbm_d6s1g4k9rgfQfmuFV_bXR6Ts31zA3PKougkSACGc26Fz-m6fs7DynegSltTTUn6ybwBFfWrku-qmdleujGD9yPYF7Rg64FAFVorN-NYRvM0-KLHjfe361b0tg4a92r8icbOJbyL5tJdZNJmkzdUnWy4MzqzEerYp123n2j6PXFaQ_Ede_6Nt7AlMda5t7s6Apj9MneP_xk8gVytU7PwjJcxBcPKpr8OBM76PIjlV-NWJ6JsE1faNrmVIzfizlwtrIRjhfGHp2K_daFaEwq9lt3ov8A`)

// Challenge that was issued to the client
var encodedChallengeData = []byte(`4lhoEqr48g2ivoOLgUW5EZPhpFlede3ucn1EcWGfVNI=`)

func getAttestationCbor() *attestationCbor {
	var decodedAttestationPayload = make([]byte, base64.URLEncoding.DecodedLen(len(encodedAttestation)))
	if _, decodeErr := base64.URLEncoding.Decode(decodedAttestationPayload, encodedAttestation); decodeErr != nil {
		slog.Error("cannot decode payload", "error", decodeErr)
		return nil
	}

	// iOS implements level 5 zlib compression - https://developer.apple.com/documentation/compression/compression_zlib
	// iOS doesn't prepend the zlib magic bytes so we're adding them, found via https://stackoverflow.com/a/43170354
	decodedAttestationPayload = append([]byte{0x78, 0x5E}, decodedAttestationPayload...) // iOS doesn't prepend the 78 and 5E bytes

	reader, readerErr := zlib.NewReader(bytes.NewBuffer(decodedAttestationPayload))
	if readerErr != nil {
		slog.Error("cannot create reader", "error", readerErr)
		return nil
	}

	var attestationCborData bytes.Buffer
	_, _ = io.Copy(&attestationCborData, reader)
	_ = reader.Close()

	//fmt.Println(hex.EncodeToString(attestationCborData.Bytes()))

	var attestation attestationCbor
	if err := cbor.Unmarshal(attestationCborData.Bytes(), &attestation); err != nil {
		slog.Error("cannot read CBOR data", "error", err)
		return nil
	}

	attestation.AttAuthData.RPIDHash = attestation.RawAuthData[:32]
	attestation.AttAuthData.Flags = AuthenticatorFlags(attestation.RawAuthData[32])
	attestation.AttAuthData.Counter = binary.BigEndian.Uint32(attestation.RawAuthData[33:37])

	attestation.AttAuthData.AttData.AAGUID = attestation.RawAuthData[37:53]
	idLength := binary.BigEndian.Uint16(attestation.RawAuthData[53:55])
	attestation.AttAuthData.AttData.CredentialID = attestation.RawAuthData[55 : 55+idLength]
	attestation.AttAuthData.AttData.CredentialPublicKey = attestation.RawAuthData[55+idLength:]

	//minAuthDataLength := 37
	//remaining := len(attestation.RawAuthData) - minAuthDataLength

	// Apple didn't read the W3C specification properly and sets the attestedCredentialData flag, while it's not present for an assertion. We'll just look a the length...
	//if len(attestation.RawAuthData) > minAuthDataLength {
	//	a.unmarshalAttestedData(attestation.RawAuthData)
	//	attDataLen := len(attestation.AttAuthData.AttData.AAGUID) + 2 + len(attestation.AttAuthData.AttData.CredentialID) + len(attestation.AttAuthData.AttData.CredentialPublicKey)
	//	remaining = remaining - attDataLen
	//}
	//
	//if remaining != 0 {
	//	return utils.ErrBadRequest.WithDetails("Leftover bytes decoding AuthenticatorData")
	//}

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
			return
		}
		credCert = _credCert
	}

	// Step 2.
	// Create clientDataHash as the SHA256 hash of the one-time challenge your server sends to your app before
	// performing the attestation, and append that hash to the end of the authenticator data
	// (authData from the decoded object).
	var clientDataHash [32]byte
	{
		//clientDataHasher := sha256.New()
		//clientDataHasher.Write(attestation.RawAuthData)
		//clientDataHasher.Write([]byte("HelloWorld"))
		//clientDataHash = clientDataHasher.Sum(nil)

		clientDataHash = sha256.Sum256([]byte("HelloWorld"))
	}

	// Step 3.
	// Generate a new SHA256 hash of the composite item to create nonce.
	var nonce [32]byte
	{
		//nonceHasher := sha256.New()
		//nonceHasher.Write(attestation.RawAuthData)
		//nonceHasher.Write(clientDataHash[:])
		//nonce = nonceHasher.Sum(nil)

		nonce = sha256.Sum256(append(attestation.RawAuthData, clientDataHash[:]...))
	}

	// Step 4.
	// Obtain the value of the credCert extension with OID 1.2.840.113635.100.8.2, which is a DER-encoded ASN.1
	// sequence. Decode the sequence and extract the single octet string that it contains.
	// Verify that the string equals nonce.
	{
		var attExtBytes []byte
		for _, ext := range credCert.Extensions {
			if ext.Id.Equal(asn1.ObjectIdentifier{1, 2, 840, 113635, 100, 8, 2}) {
				attExtBytes = ext.Value
			}
		}
		if len(attExtBytes) == 0 {
			slog.Error("Attestation certificate extensions missing 1.2.840.113635.100.8.2")
			return
		}

		decoded := AppleAnonymousAttestation{}
		if _, err := asn1.Unmarshal(attExtBytes, &decoded); err != nil {
			slog.Error("Unable to parse apple attestation certificate extensions")
			return
		}

		if !bytes.Equal(decoded.Nonce, nonce[:]) {
			slog.Error("Attestation certificate does not contain expected nonce")
			return
		}
	}

	// Step 5.
	// Create the SHA256 hash of the public key in credCert, and verify that it matches the key identifier from
	// your app.
	var publicKey *ecdsa.PublicKey
	{
		pubKey := credCert.PublicKey.(*ecdsa.PublicKey)
		publicKeyBytes := elliptic.Marshal(pubKey, pubKey.X, pubKey.Y)
		pubKeyHash := sha256.Sum256(publicKeyBytes)
		keyID, _ := base64.URLEncoding.DecodeString("XhA41blm3ysDPvR0o8Kv1x2FXwIBgdBt7GCpJ7IgCgM=") // TODO: don't hardcode
		if !bytes.Equal(pubKeyHash[:], keyID) {
			slog.Error("Mismatch")
			return
		}
		publicKey = pubKey
	}
	_ = publicKey

	// Step 6.
	// Compute the SHA256 hash of your app’s App ID, and verify that it’s the same as the authenticator
	// data’s RP ID hash.
	{
		// TODO where to get the authenticator data from?
		appID := sha256.Sum256([]byte("5MRWH833JE.com.muzmatch.muzmatch.alpha"))
		if !bytes.Equal(appID[:], attestation.AttAuthData.RPIDHash) {
			slog.Error("RPID mismatch")
			return
		}
	}

	// Step 7.
	// Verify that the authenticator data’s counter field equals 0.
	{
		if attestation.AttAuthData.Counter != 0 {
			slog.Error("authenticator data counter field does not equal 0.")
			return
		}
	}

	// Step 8.
	// Verify that the authenticator data’s aaguid field is either appattestdevelop if operating in the
	// development environment, or appattest followed by seven 0x00 bytes if operating in the production environment.
	// TODO: prod is different
	{
		if !bytes.Equal([]byte("appattestdevelop"), attestation.AttAuthData.AttData.AAGUID) {
			slog.Error("bad AAGUID provided")
			return
		}
	}

	// Step 9.
	// Verify that the authenticator data’s credentialId field is the same as the key identifier.
	{
		keyID, _ := base64.URLEncoding.DecodeString("XhA41blm3ysDPvR0o8Kv1x2FXwIBgdBt7GCpJ7IgCgM=") // TODO: don't hardcode
		if !bytes.Equal(keyID, attestation.AttAuthData.AttData.CredentialID) {
			slog.Error("bad credentialID provided")
			return
		}
	}

	fmt.Println("DONE")
}

type attestationCbor struct {
	AttAuthData appleAuthData
	Fmt         string           `cbor:"fmt"`
	AttStmt     appleCborAttStmt `cbor:"attStmt"`
	RawAuthData []byte           `cbor:"authData"`
}

type appleCborAttStmt struct {
	X5C     [][]byte `cbor:"x5c"`
	Receipt []byte   `cbor:"receipt"`
}

type AuthenticatorFlags byte

type AttestedCredentialData struct {
	AAGUID       []byte `cbor:"aaguid"`
	CredentialID []byte `cbor:"credentialId"`
	// The raw credential public key bytes received from the attestation data
	CredentialPublicKey []byte `cbor:"public_key"`
}

type appleAuthData struct {
	RPIDHash []byte                 `cbor:"rpid"`
	Flags    AuthenticatorFlags     `cbor:"flags"`
	Counter  uint32                 `cbor:"sign_count"`
	AttData  AttestedCredentialData `cbor:"att_data"`
	ExtData  []byte                 `cbor:"ext_data"`
}

// Apple has not yet publish schema for the extension(as of JULY 2021.)
type AppleAnonymousAttestation struct {
	Nonce []byte `asn1:"tag:1,explicit"`
}

// type appleCborCert struct {
// CredCert []byte `cbor:"credCert"`
// CACert   []byte `cbor:"caCert"`
// }
