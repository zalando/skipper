package main

import (
	"context"
	_ "embed"
	"fmt"

	"google.golang.org/api/option"
	"google.golang.org/api/playintegrity/v1"
)

var (
	//go:embed googleCredentials.json
	googleCredentials []byte
)

type googlePlayIntegrityServiceClient struct {
	client *playintegrity.Service
}

func newGooglePlayIntegrityServiceClient() googlePlayIntegrityServiceClient {
	client, initGoogleServiceErr := playintegrity.NewService(
		context.Background(),
		option.WithCredentialsJSON(googleCredentials),
	)
	if initGoogleServiceErr != nil {
		panic("Failed to init Google Play Integrity Service")
	}

	return googlePlayIntegrityServiceClient{
		client: client,
	}
}

func (c googlePlayIntegrityServiceClient) validate(token []byte, nonce string) integrityEvaluation {
	googleResponse, googleErr := c.client.
		V1.
		DecodeIntegrityToken(
			productionAndroidPackageName,
			&playintegrity.DecodeIntegrityTokenRequest{
				IntegrityToken: string(token),
			},
		).
		Do()
	if googleErr != nil {
		// $appAttestation->setGoogleResponse(json_encode($e->getErrors()));
		// $appAttestation->setPlatformSuccess(false);
		// $appAttestation->setMuzzError("Google threw an exception");
		return integrityFailure
	}

	// $appAttestation->setGoogleResponse((string)json_encode($response));

	appVerdict := googleResponse.TokenPayloadExternal.AppIntegrity.AppRecognitionVerdict
	if appVerdict == "UNEVALUATED" {
		// $appAttestation->setPlatformSuccess(false);
		// $appAttestation->setMuzzError("Google app verdict is UNEVALUATED");
		return integrityUnevaluated
	}

	deviceVerdict := googleResponse.TokenPayloadExternal.DeviceIntegrity.DeviceRecognitionVerdict
	certSha256Digest := googleResponse.TokenPayloadExternal.AppIntegrity.CertificateSha256Digest[0]
	requestPackageName := googleResponse.TokenPayloadExternal.RequestDetails.RequestPackageName
	googleNonce := googleResponse.TokenPayloadExternal.RequestDetails.Nonce

	var muzzError []string

	// Check if the signing certificate is invalid
	var certDigestMatch bool
	for _, certDigest := range []string{
		productionAndroidSigningCertDigest,
		debugAndroidSigningCertDigest,
	} {
		if certSha256Digest == certDigest {
			certDigestMatch = true
		}
	}
	if !certDigestMatch {
		muzzError = append(muzzError, "Invalid Android CertificateSha256Digest: "+certSha256Digest)
	}

	if requestPackageName != productionAndroidPackageName && requestPackageName != debugAndroidPackageName {
		muzzError = append(muzzError, "Invalid Android RequestPackageName: "+requestPackageName)
	}

	// Did Google give us confidence in the installation and device?
	var platformSuccess = true
	_ = platformSuccess

	// Ensure the app has been recognised by the Play Store the package is `com.muzmatch.muzmatchapp`
	if requestPackageName != productionAndroidPackageName {
		if appVerdict != "PLAY_RECOGNIZED" {
			platformSuccess = false
			muzzError = append(muzzError, "Invalid AppRecognitionVerdict: "+appVerdict)
		}
	}

	if deviceVerdict[0] != "MEETS_DEVICE_INTEGRITY" {
		platformSuccess = false
		muzzError = append(muzzError, "Invalid DeviceRecognitionVerdict: "+deviceVerdict[0])
	}

	// Are the nonce values the same?
	var nonceSuccess = true
	if nonce != googleNonce {
		nonceSuccess = false
		muzzError = append(muzzError, fmt.Sprintf("Nonce mismatch: server %q app %q", nonce, googleNonce))
	}
	_ = nonceSuccess

	// $appAttestation->setPlatformSuccess($platformSuccess);
	// $appAttestation->setNonceSuccess($nonceSuccess);

	if len(muzzError) > 0 {
		// $appAttestation->setMuzzError(implode("\n", $muzzError));
		return integrityFailure
	}

	return integritySuccess
}
