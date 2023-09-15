package main

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"golang.org/x/mod/semver"
	"io"
	"net/http"
	"strings"

	"github.com/zalando/skipper/filters"
)

var _ filters.Filter = (*attestationFilter)(nil)

type attestationFilter struct {
	repo       *repo
	googlePlay googlePlayIntegrityServiceClient
	appStore   appStore
}

func (a attestationFilter) Request(ctx filters.FilterContext) {
	r := ctx.Request()

	uri := r.URL.RequestURI()
	var isProtectedRoute bool
	for _, protectedRoute := range []string{
		"/v2.5/auth/confirm",
	} {
		if uri == protectedRoute {
			isProtectedRoute = true
			break
		}
	}

	if !isProtectedRoute {
		// Not a protected route, skip
		return
	}

	// Fetch headers we'll need
	deviceUDID := r.Header.Get("udid")
	userAgent := r.Header.Get("user-agent")
	appVersion := r.Header.Get("appVersion")
	authorizationHeader := r.Header.Get("authorization")
	bypassHeader := r.Header.Get("x-muzz-bypass-device-integrity-check")
	encodedKeyId := r.Header.Get("x-keyid")             // iOS only
	encodedAssertation := r.Header.Get("x-assertation") // iOS only

	feature := strings.Contains(r.Header.Get("features"), "SUPPORTS_CHALLENGE_RESPONSE")
	if !feature {
		bypassHeader = "true"
	}

	// Determine platform
	var isAndroid = androidUserAgent.MatchString(r.Header.Get("user-agent"))
	var isIOS bool
	for _, rgx := range iOSUserAgents {
		if rgx.MatchString(userAgent) {
			isIOS = true
			break
		}
	}

	// Check there is a UDID
	if deviceUDID == "" {
		sendErrorResponse(ctx, http.StatusForbidden, "Missing UDID in request")
		return
	}

	// Check there is an app version
	if appVersion == "" {
		sendErrorResponse(ctx, http.StatusForbidden, "Missing app version in request")
		return
	}

	// Enforce minimum versions of apps
	var platform Platform
	switch {
	case isAndroid:
		platform = PlatformAndroid

		if !strings.HasPrefix(appVersion, "v") {
			appVersion = "v" + appVersion
		}

		// It always does, but just to be safe
		if strings.HasSuffix(appVersion, "a") {
			appVersion = strings.TrimSuffix(appVersion, "a")
		}

		// TODO: the body is a bit trickier, plus needs to be localised
		if semver.Compare(appVersion, minimumAndroidVersion) < 0 {
			//sendErrorResponse(ctx, http.StatusUpgradeRequired, "Invalid OS")
		}

		// skip android for now
		bypassHeader = "true"
	case isIOS:
		platform = PlatformIos

		if !strings.HasPrefix(appVersion, "v") {
			appVersion = "v" + appVersion
		}

		// TODO: the body is a bit trickier, plus needs to be localised
		if semver.Compare(appVersion, minimumIosVersion) < 0 {
			//sendErrorResponse(ctx, http.StatusUpgradeRequired, "Invalid OS")
			bypassHeader = "true"
		}
	default:
		sendErrorResponse(ctx, http.StatusForbidden, "Invalid OS")
		return
	}

	// Is there a bypass header (used for automated tests and in Postman)?
	if bypassHeader != "" {
		return
	}

	// TODO: error handling
	existingAppAttestation, _ := a.repo.GetAttestationForUDID(deviceUDID)

	// If there is no authorization header, or there is no existing app attestation record in the database, issue the challenge
	if existingAppAttestation == nil || authorizationHeader == "" {
		// Generate 128 random bytes
		buf := make([]byte, 128)
		_, _ = rand.Read(buf)

		requestBody, _ := io.ReadAll(ctx.Request().Body)

		err := a.repo.CreateAttestationForUDID(
			deviceUDID,
			[]byte(base64.URLEncoding.EncodeToString(buf)),
			platform,
			ctx.Request().Header,
			string(requestBody),
		)
		if err != nil {
			return
		}

		header := http.Header{}
		header.Set("Content-Type", "application/json")
		header.Set("WWW-Authenticate", "Integrity")

		b, _ := json.Marshal(
			struct {
				Challenge string `json:"challenge"`
			}{
				Challenge: base64.URLEncoding.EncodeToString(buf),
			},
		)

		ctx.Serve(
			&http.Response{
				StatusCode: 480, // 480 is the response we've agreed with the apps teams to initiate integrity check
				Header:     header,
				Body:       io.NopCloser(bytes.NewBufferString(string(b))),
			},
		)
		return
	}

	// Authorization header is present, lets validate
	if !strings.HasPrefix(authorizationHeader, "Integrity ") {
		sendErrorResponse(ctx, http.StatusForbidden, "Missing integrity authorization header")
		return
	}
	authorizationHeader = strings.TrimPrefix(authorizationHeader, "Integrity ")

	// Check for empty authorization header
	if authorizationHeader == "" {
		sendErrorResponse(ctx, http.StatusForbidden, "Empty authorization header")
		return
	}

	// Set the challenge response we received
	existingAppAttestation.ChallengeResponse = authorizationHeader
	a.repo.UpdateAttestationForUDID(existingAppAttestation)

	// Has the app sent an error code instead
	if isIOS {
		switch authorizationHeader {
		case "serverUnavailable":
		case "unknownSystemFailure":
			existingAppAttestation.DeviceErrorCode = authorizationHeader
			a.repo.UpdateAttestationForUDID(existingAppAttestation)

			// TODO: issue a captcha challenge
			return
		}
	}

	if isAndroid {
		switch authorizationHeader {
		case "API_NOT_AVAILABLE":
		// Make sure that Integrity API is enabled in Google Play Console.
		// Ask the user to update Google Play Store.
		case "NETWORK_ERROR": // Ask them to retry
		case "PLAY_STORE_NOT_FOUND": // Ask the user to install or enable Google Play Store.
		case "PLAY_STORE_VERSION_OUTDATED": // Ask the user to update Google Play Store.
		case "PLAY_STORE_ACCOUNT_NOT_FOUND": // Ask the user to sign in to the Google Play Store.
		case "CANNOT_BIND_TO_SERVICE": // Ask the user to update the Google Play Store.
		case "PLAY_SERVICES_NOT_FOUND": // Ask the user to install or enable Play Services.
		case "PLAY_SERVICES_VERSION_OUTDATED": // Ask the user to update Google Play services.
		case "TOO_MANY_REQUESTS": // Retry with an exponential backoff.
		case "GOOGLE_SERVER_UNAVAILABLE": // Retry with an exponential backoff.
		case "CLIENT_TRANSIENT_ERROR": // Retry with an exponential backoff.
		case "INTERNAL_ERROR": // Retry with an exponential backoff.
		case "APP_NOT_INSTALLED": // Pass error to API and do nothing else
		case "NONCE_TOO_SHORT": // Pass error to API and do nothing else
		case "NONCE_TOO_LONG": // Pass error to API and do nothing else
		case "NONCE_IS_NOT_BASE64": // Pass error to API and do nothing else
		case "CLOUD_PROJECT_NUMBER_IS_INVALID": // Pass error to API and do nothing else
		case "APP_UID_MISMATCH": // Pass error to API and do nothing else
		// The following are catch-all errors reported by the client
		case "INVALID_ERROR": // Google client SDK returned an error but didn't match an expected error code
		case "ERROR": // There was some non-Google SDK error that stopped authorization being granted
			existingAppAttestation.DeviceErrorCode = authorizationHeader
			a.repo.UpdateAttestationForUDID(existingAppAttestation)

			// TODO: issue a captcha challenge
			return
		}
	}

	// Base64 decode the header value
	challengeResponse, base64decodeErr := base64.URLEncoding.DecodeString(authorizationHeader)
	if base64decodeErr != nil {
		sendErrorResponse(ctx, http.StatusForbidden, "Could not decode challenge response from base64 URL encoding")
		return
	}

	// Calculate the hash
	var base64encodedChallenge string // TODO: base64.URLEncoding.EncodeToString(existingAppAttestation.challenge))
	serverNonce, serverNonceErr := calculateRequestNonce(ctx.Request(), base64encodedChallenge)
	if serverNonceErr != nil {
		sendErrorResponse(ctx, http.StatusInternalServerError, "Failed to calculate server nonce")
		return
	}

	switch {
	case isAndroid:
		verdict := a.googlePlay.validate(challengeResponse, serverNonce)
		a.repo.UpdateAttestationForUDID(existingAppAttestation)

		if verdict == integritySuccess {
			return // All good, proceed
		}

		if verdict == integrityUnevaluated {
			// TODO: Captcha challenge
			return
		}

		// Integrity failed, throw an error
		sendErrorResponse(ctx, http.StatusForbidden, "Integrity check failed")

	case isIOS:
		if encodedAssertation == "" {
			sendErrorResponse(ctx, http.StatusForbidden, "Empty x-assertation header")
			return
		}
		if encodedKeyId == "" {
			sendErrorResponse(ctx, http.StatusForbidden, "Empty x-keyid header")
			return
		}

		verdict := a.appStore.validate(authorizationHeader, existingAppAttestation.Challenge, encodedKeyId)
		a.repo.UpdateAttestationForUDID(existingAppAttestation)

		if verdict == integritySuccess {
			return // All good, proceed
		}

		if verdict == integrityUnevaluated {
			// TODO: Captcha challenge
			return
		}

		// Integrity failed, throw an error
		sendErrorResponse(ctx, http.StatusForbidden, "Integrity check failed")
	}

	// All good, continue
	return
}

func (a attestationFilter) Response(_ filters.FilterContext) {}
