package main

import (
	_ "embed"
	"encoding/base64"
	"github.com/zalando/skipper/plugins/filters/attestation/ios"
	"log/slog"
)

//go:embed Apple_App_Attestation_Root_CA.pem
var appleRootCertBytes []byte

// This comes from the device and is sent as the header - it is ZLIB encoded
var encodedAttestation = `7VgJVBPXGk4mQ9gioCgIbuNSZA13MgkkUJUUERARZJHFCg6ThWg2k0HE1kqionBKa12qtLaCqBW1Yl1oFYFXLc_dVsXW5SGlKmIfVG2pipb67hDA4LPbOT2v57zTnNy5-W_-u-T_v-___5tySqGhdaRer5YHwCdJ03IjrYRdIq2hN1MLRZQ5DbkLzEgbbIZSDsJGEC67aFmZ4xngwLXzLYg6OwHlICAOHwtGcznJKIfnKWVWw-ATk3YvR9IqnRYLl2I47goGMkoOPAeLUrSW4vcO2vEcwkm1SqEzaFUkGOk-QEAACQ5woUAowNN7RKJHBKa38GgQadkxTCTHASGWibIkQUEyhSALEIRcIQwWkgQlIBWyYFwmFokUQABwMS4DQTI5FQRIiSA4SyAAgAQE7gmGMks58gZKpVIsXG6gVQoV1X3wP3DmNODKte02CcLusQ2Hbct5iYU2T2xK8Dzgduj7iyF3P508-7x_0d4Hre7ml7FIdFDllyk-FfPrc2JO-r5dUI3dKBE06zcuT-VNq-AFs0Js53zB2_Sw3HQfmH4APLjnCFc2-wmKABZwYiRnRoIeQO8CUz7XHm5a8ICS2dmgRjB_C3SXQ10hgB27rhCHHauuUGCRCEsn9Ea9RLEJKVFigpgawad0Gr4mZ5GGpKnspx9ItT6b3MpFUeM8I1ZXGASn2tQVBlvWk1i6EKYDw56ewBZ1Ary6ooV2KBcX8oV8ASCefomgXmBc2RgUi5hkOt3pTJaU2R-b7zbyeGROqFtWwXrR7PU3qJkjR4_63BpknGwWkCMgu6Ux7nHGuMwE9wfNidKuCNS94thB6sFjn8Ilc_elV7MXv9eYGN-xsnCFKJnbkl9LITir9R9RL34TUKzSNlz38Ll21n_xYdcM9fBYm-avZ611OFZJfFcqzvJwkPK_Ozokk7djpnMaEgnhHg7M7OMWyLvYH6pvPhzmebkhcd1hsaddktW5OCABh7_GAsURzwd_gk5HQwb8ASwx-AcQmWJcTEhEIoh_ghHhg3ml__mEWwBcnoLXxs-EssZwsljo7lnBpbmxeXtFb4bdOve9l9e9I5l33PMDvt1nnpu2Iu7h-Q-HvLDwnc0K1GNQVNWLB5Xc9zdcoGoygp3rSt8QvJ9vrFh-i9aWp22-r_O5KqiMyBjtlnpRWfm-12t7ydgjnoqzJeUKIAOD-nBtB7iwY6A0ihkbiw4FbvmDd73lkkjU1GZLn3zAq7j56NubN4vKwAhGwQl1QwdPvD57OOpRcWI__nAZ_WVzc4rh-rv92cHmWjuMo2IBBYRFde3KYmOxDfL54S6Fxj4lXnT4yxX-k5aBaC-f9nOdg1vzSoa_03pg98ML25OIB1UDuHDKTnH7F6dN7awtCUcPTe5cQ8kTfz7ts4S_v2lTRm361JNVJS_HmAd0omLdQtvq1LWSj5UGOSVX6ek0pyzQS88BbFukNB_kQwLizmAA135OQRRbzkEhq1jWSuzS_HH5qJnTipvREYDP4JDN_r10hfh9hLAZkqPmvwP47zzz3wH8_ySAA2-EjTLYx1bwp8efLVl1aY-U4EWeaDw3rG1efMht8dySK8uX3a4e9BmYA23BaKbGRk2VSYSGZKFSlTYlUBmdNyOKjAkXRsULDMoF8mRNnEoar5YvSs2ZlzgzWxsjnqbG4_OCg7SSmdGKvFxp4NxwuVChShQqQUR2Skx8unJ6oF_kjAkTgBMEE7MDV5qUFJGYBJwRGACgbGsktbIs3UIwCmHzmAFU4i4AAiIASAJwPAkXhgjgG-cLJemMyhBGxaKACwJw0E-B1f2CUcXM-RC2pJ60tfhMyYyA1xefcXO85iQ2LXqpH-VfxQHgWwg4vo_y6h7WQJbQcqWhj_oiLACLxHsTnWNvouvHNEyaQ2frDCo67_mccwT2zCDXFUlO7IkPBBDjIoEED2bigxAIQVC3GJQO0vEgILScLsD6XNYhaYqBzJFhCZYQiyWqlFqVVvk79v5lnl_Z4K28M_u8z65hNbUb59u3T_oqZqtH3tA9k9WegsCWmSnRbZ0fVV9EjYi6Ze2lEReO2zWaXxu579Tje12-Pt8scekqN7MvwbKh4RmmWyezy-4_xyhXiWNWd1w9ezElf7DjmxNOgHCunR-XbWNjC30cDESQaj0yYBeMz6ZpfUhgoI4y6vndpTpD824REAEkCS0jUsI4C_cdzuyDoWa2KxScYXPso6wNG5i6gOlo78IIAkxVPNO-BLlaRWopOQbtSWerjBjV51I5lpWHkdo8TE8aaPjJaMzRyI0YSVFyPW2Zo4Bz5EzTYqTFSVnQ4tA_WhlpkGG03KCBE7QyjNJpZSrGaUZmUo5R7t9vI70Ozs2zaPaDlN5AUrSK6l6TlmvkWtrIB6K-H8F28-mxTm5urpVxrNYme0FpVTFMu_fBJheyyFMiO61puRpqm7V-QU3_cGybD5z7bMdzRhGYm61DWSQLTEawRVdl3-RsjLNxe-IfffWHVf6BA_235b4Rb3agl27cFfpuG4JFiI7N8RWExXb6mpQ_ra8yfG1f0uBT9PYtm1Pt9nr3-GKYlTthW9JD2JmPlzb4PSk-Glwy9kD6bm1NvzpTiQ8DHhZKDLJAu6euZLhJ_LncxCWAEAjwYBFBEEzxKWTE3uLzr40cv8ze1WepmsUFH-30dggd3lz5GF2b_9au-3jTtlPeIt3kmObRN1MaBxUXHzK_uq42-lEtcXL3GdTjXmD4C486avedWvraoXLTA2DqgI7v5a4N4MCuH32r91wrSyVWvrOlqORabc2_K6-cGrcTTLGibwgQgyAr-vr-On2ZAQN0I0UqCRDMbDQKhdYF_qW-pd4FXj2TKYPaam6_SXz4nRW0nxteni2GXbgOPdDmIpxnsM2xpGmcVazbtqns6xtts6Ybi1YvmbAwccObHVdGpY48mjP1Z_H2ab5LPT13DNgm0zQfXNN6LnlaOwKUof77m1uDbXNPXWxbsN0zJDl0kmju8fqW1Xuaydr8aNWNTL8KdfNAZMdp8rL4Y-PUnkvWCQv47QLqf1p5vj5m3V8HepiBCKbGlIAgBvQSK_F_d5BfuIltuB04IcywpX3NGIPJdXijy7p79eXT5_9rsNtlp5vbbHxzZa9kThdfqB4--cCSH0dVCbJmn7i9bFYciBuYzupKTeVt_dHZP9Y20jXnyvr163X4y-1-772ChV46mHQ5o2RLyAwHvPwlEGaFo-fi_L-Y8Wu3rB4YLb396WduGZ6zhUMvt9_pUDuxpsR80rR3ysB6566WRa-XbZt39ginYeOPa_5JfzK3acfYIQjQKIpcTjYMaIg7Uxw-cX8RtZXQXtdccf9YNup4olfnyixa4ug-69SM7bzWoZxbm_d6s1g4k9rgfQfmuFV_bXR6Ts31zA3PKougkSACGc26Fz-m6fs7DynegSltTTUn6ybwBFfWrku-qmdleujGD9yPYF7Rg64FAFVorN-NYRvM0-KLHjfe361b0tg4a92r8icbOJbyL5tJdZNJmkzdUnWy4MzqzEerYp123n2j6PXFaQ_Ede_6Nt7AlMda5t7s6Apj9MneP_xk8gVytU7PwjJcxBcPKpr8OBM76PIjlV-NWJ6JsE1faNrmVIzfizlwtrIRjhfGHp2K_daFaEwq9lt3ov8A`

// Challenge that was issued to the client
var encodedChallengeData = `orfwoR3T6AELz5Tc0CrTZ-PPnMXVUweMV8tLbs55i0mL8KoHvyFoSRjM4B1CKMiwCTcEQeAsBtjuSQnOOpdsJCzLy4M8V7gY2oMG3N3Q-lgHRvCIon3LOdleGmoW90M6ig0azUijhxJogNVcHhZSC748SGKEajHvsh8UYck7IF4=`

var encodedKeyID = `XhA41blm3ysDPvR0o8Kv1x2FXwIBgdBt7GCpJ7IgCgM=`

func buildRequest(
	appleRootCertBytes []byte,
	encodedAttestation string,
	encodedChallengeData string,
	encodedKeyID string,
) (*ios.AttestationRequest, error) {
	var req ios.AttestationRequest
	req.RootCert = appleRootCertBytes

	decodedAttestationPayload, err := base64.URLEncoding.DecodeString(encodedAttestation)
	slog.Debug("attestation payload", "payload", encodedAttestation)
	if err != nil {
		slog.Error("cannot decode attestation payload", "error", err)
		return nil, err
	}
	req.DecodedAttestation = decodedAttestationPayload // Still in ZLIB format

	decodedChallengeData, err := base64.URLEncoding.DecodeString(encodedChallengeData)
	slog.Debug("challenge data payload", "payload", encodedChallengeData)
	if err != nil {
		slog.Error("cannot decode challenge data payload", "error", err)
		return nil, err
	}
	req.DecodedChallengeData = decodedChallengeData

	decodedKeyID, err := base64.URLEncoding.DecodeString(encodedKeyID)
	slog.Debug("key id payload", "payload", encodedKeyID)
	if err != nil {
		slog.Error("cannot decode key id payload", "error", err)
		return nil, err
	}
	req.DecodedKeyID = decodedKeyID

	return &req, nil
}

func main() {
	req, err := buildRequest(
		appleRootCertBytes,
		encodedAttestation,
		encodedChallengeData,
		encodedKeyID,
	)
	if err != nil {
		slog.Error("bad request", "err", err)
		return
	}

	attestation := ios.New(req)

	if err = attestation.Parse(); err != nil {
		slog.Error("parse fail", "err", err)
		return
	}

	// Step 1.
	// Verify that the x5c array contains the intermediate and leaf certificates for App Attest,
	// starting from the credential certificate in the first data buffer in the array (credcert).
	// Verify the validity of the certificates using Apple's App Attest root certificate.
	if err = attestation.ValidateCertificate(); err != nil {
		slog.Error("validate certificate", "err", err)
		return
	}

	// Step 2.
	// Create clientDataHash as the SHA256 hash of the one-time challenge your server sends to your app before
	// performing the attestation, and append that hash to the end of the authenticator data
	// (authData from the decoded object).
	attestation.ClientHashData()

	// Step 3.
	// Generate a new SHA256 hash of the composite item to create nonce.
	attestation.GenerateNonce()

	// Step 4.
	// Obtain the value of the credCert extension with OID 1.2.840.113635.100.8.2, which is a DER-encoded ASN.1
	// sequence. Decode the sequence and extract the single octet string that it contains.
	// Verify that the string equals nonce.
	if err = attestation.CheckAgainstNonce(); err != nil {
		slog.Error("check against nonce", "err", err)
		return
	}

	// Step 5.
	// Create the SHA256 hash of the public key in credCert, and verify that it matches the key identifier from your app.
	if err = attestation.GeneratePublicKey(); err != nil {
		slog.Error("generate public key", "err", err)
		return
	}

	// Step 6.
	// Compute the SHA256 hash of your app's App ID, and verify that it's the same as the authenticator
	// data's RP ID hash.
	if err = attestation.CheckAgainstAppID(); err != nil {
		slog.Error("check against appID", "err", err)
		return
	}

	// Step 7.
	// Verify that the authenticator data’s counter field equals 0.
	if err = attestation.CheckCounterIsZero(); err != nil {
		slog.Error("check counter is zero", "err", err)
		return
	}

	// Step 8.
	// Verify that the authenticator data’s aaguid field is either appattestdevelop if operating in the
	// development environment, or appattest followed by seven 0x00 bytes if operating in the production environment.
	if err = attestation.ValidateAAGUID(); err != nil {
		slog.Error("validate AAGUID", "err", err)
		return
	}

	// Step 9.
	// Verify that the authenticator data’s credentialId field is the same as the key identifier.
	if err = attestation.ValidateCredentialID(); err != nil {
		slog.Error("validate credentialID", "err", err)
		return
	}

	slog.Info("Complete!")
}
