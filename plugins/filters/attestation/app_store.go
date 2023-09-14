package main

import (
	_ "embed"
	"encoding/base64"
	"github.com/zalando/skipper/plugins/filters/attestation/ios"
	"log/slog"
)

var (
	//go:embed Apple_App_Attestation_Root_CA.pem
	appleRootCertBytes []byte
)

type appStore struct {
	req *ios.AttestationRequest
}

func newAppStoreIntegrityServiceClient() appStore {
	return appStore{}
}

func (as appStore) buildRequest(
	encodedAttestation string,
	challengeData []byte,
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

	req.ChallengeData = challengeData

	decodedKeyID, err := base64.StdEncoding.DecodeString(encodedKeyID)
	slog.Debug("key id payload", "payload", encodedKeyID)
	if err != nil {
		slog.Error("cannot decode key id payload", "error", err)
		return nil, err
	}
	req.DecodedKeyID = decodedKeyID

	return &req, nil
}

func (as appStore) validate(
	encodedAttestation string,
	encodedChallengeData []byte,
	encodedKeyID string,
) integrityEvaluation {
	req, err := as.buildRequest(
		encodedAttestation, encodedChallengeData, encodedKeyID,
	)
	if err != nil {
		slog.Error("bad request", "err", err)
		return integrityFailure
	}

	attestation := ios.NewAttestation(req)

	if err = attestation.Parse(); err != nil {
		slog.Error("parse fail", "err", err)
		return integrityFailure
	}

	// Step 1.
	// Verify that the x5c array contains the intermediate and leaf certificates for App Attest,
	// starting from the credential certificate in the first data buffer in the array (credcert).
	// Verify the validity of the certificates using Apple's App Attest root certificate.
	if err = attestation.ValidateCertificate(); err != nil {
		slog.Error("validate certificate", "err", err)
		return integrityFailure
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
		return integrityFailure
	}

	// Step 5.
	// Create the SHA256 hash of the public key in credCert, and verify that it matches the key identifier from your app.
	if err = attestation.GeneratePublicKey(); err != nil {
		slog.Error("generate public key", "err", err)
		return integrityFailure
	}

	// Step 6.
	// Compute the SHA256 hash of your app's App ID, and verify that it's the same as the authenticator
	// data's RP ID hash.
	if err = attestation.CheckAgainstAppID(); err != nil {
		slog.Error("check against appID", "err", err)
		return integrityFailure
	}

	// Step 7.
	// Verify that the authenticator data’s counter field equals 0.
	if err = attestation.CheckCounterIsZero(); err != nil {
		slog.Error("check counter is zero", "err", err)
		return integrityFailure
	}

	// Step 8.
	// Verify that the authenticator data’s aaguid field is either appattestdevelop if operating in the
	// development environment, or appattest followed by seven 0x00 bytes if operating in the production environment.
	if err = attestation.ValidateAAGUID(); err != nil {
		slog.Error("validate AAGUID", "err", err)
		return integrityFailure
	}

	// Step 9.
	// Verify that the authenticator data’s credentialId field is the same as the key identifier.
	if err = attestation.ValidateCredentialID(); err != nil {
		slog.Error("validate credentialID", "err", err)
		return integrityFailure
	}

	return integritySuccess
}
