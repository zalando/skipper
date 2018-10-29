package apiusagemonitoring

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
)

const (
	authorizationHeaderName = "Authorization"
	authorizationHeaderPrefix = "Bearer "
)

// parseJwtBody parses the JWT token from a HTTP request.
// It returns `nil` if it was not possible to parse the JWT body.
func parseJwtBody(req *http.Request) map[string]interface{} {
	ahead := req.Header.Get(authorizationHeaderName)
	if !strings.HasPrefix(ahead, authorizationHeaderPrefix) {
		return nil
	}

	// split the header into the 3 JWT parts
	fields := strings.Split(ahead, ".")
	if len(fields) != 3 {
		return nil
	}

	// base64-decode the JWT body part
	sDec, err := base64.RawURLEncoding.DecodeString(fields[1])
	if err != nil {
		return nil
	}

	// un-marshall the JWT body from JSON
	var h map[string]interface{}
	err = json.Unmarshal(sDec, &h)
	if err != nil {
		return nil
	}

	return h
}
