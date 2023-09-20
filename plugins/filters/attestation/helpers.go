package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/zalando/skipper/filters"
)

type errorResponse struct {
	Error errorObj `json:"error"`
}

type errorObj struct {
	Status  int             `json:"status"`
	Details errorObjDetails `json:"details"`
}

type errorObjDetails struct {
	Message string `json:"message"`
}

func sendErrorResponse(ctx filters.FilterContext, statusCode int, message string) {
	b, _ := json.Marshal(
		errorResponse{
			Error: errorObj{
				Status: statusCode,
				Details: errorObjDetails{
					Message: message,
				},
			},
		},
	)

	header := http.Header{}
	header.Set("Content-Type", "application/json")

	ctx.Serve(
		&http.Response{
			StatusCode: statusCode,
			Header:     header,
			Body:       io.NopCloser(bytes.NewBufferString(string(b))),
		},
	)
}

func env() string {
	switch os.Getenv("ENVIRONMENT") {
	case production:
		return production
	case dev:
		return dev
	default:
		return local
	}
}

func calculateRequestNonce(r *http.Request, challenge string) (string, error) {
	r.URL.Scheme = "https"
	switch env() {
	case production:
		r.URL.Host = "api.muzzapi.com"
	case dev:
		r.URL.Host = "api.dev.muzzapi.com"
	default:
		r.URL.Host = "localhost"
	}

	usingBody := true
	bodyBuf, readBodyErr := io.ReadAll(r.Body)
	if readBodyErr != nil {
		return "", fmt.Errorf("cannot read body: %w", readBodyErr)
	}

	r.Body = io.NopCloser(bytes.NewBuffer(bodyBuf)) // Set the body back to the original so it can be read again later
	dataToHash := bytes.NewBuffer(bodyBuf).Bytes()

	// If there's no request body use the URL as the data to hash
	if len(dataToHash) == 0 {
		usingBody = false
		dataToHash = []byte(r.URL.String())
	}

	fmt.Printf("dataToHash is body data: %v\n", usingBody)
	if !usingBody {
		fmt.Printf("dataToHash: %s\n", string(dataToHash))
	}
	hash := sha256.New()
	hash.Write(dataToHash)
	hSum := hash.Sum(nil)
	fmt.Printf("sha256 of dataToHash: %x\n", hSum)
	b64 := base64.URLEncoding.EncodeToString(hSum)
	fmt.Printf("b64 of above: %s\n", b64)
	hash.Reset()
	hash.Write([]byte(base64.URLEncoding.EncodeToString([]byte(challenge)) + b64))
	hSum2 := hash.Sum(nil)
	fmt.Printf("above prepended with challenge and sha256: %x\n", hSum2)
	fmt.Printf("b64 of above: %s\n", base64.URLEncoding.EncodeToString(hSum2))

	fmt.Println(strings.Repeat("#", 80))
	return base64.URLEncoding.EncodeToString(hSum2), nil
}
