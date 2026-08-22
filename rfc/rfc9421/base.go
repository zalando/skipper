package rfc9421

import (
	"fmt"
	"net/http"
	"strings"
)

// buildSignatureBase constructs the signature base string per RFC 9421 Section 2.5.
func (s *Signer) buildSignatureBase(req *http.Request, signatureParams string) (string, error) {
	var lines []string

	for _, comp := range s.config.Components {
		cleanComp := strings.TrimSpace(comp)
		if cleanComp == "" {
			continue
		}

		val, err := s.extractComponentValue(req, cleanComp)
		if err != nil {
			return "", err
		}

		lines = append(lines, fmt.Sprintf("%q: %s", strings.ToLower(cleanComp), val))
	}

	lines = append(lines, fmt.Sprintf("\"@signature-params\": %s", signatureParams))

	return strings.Join(lines, "\n"), nil
}

// extractComponentValue extracts the canonicalized component value according to RFC 9421.
func (s *Signer) extractComponentValue(req *http.Request, component string) (string, error) {
	lower := strings.ToLower(component)

	switch lower {
	case "@method":
		method := req.Method
		if method == "" {
			method = http.MethodGet
		}
		return method, nil

	case "@path":
		if req.URL != nil && req.URL.EscapedPath() != "" {
			return req.URL.EscapedPath(), nil
		}
		if req.URL != nil && req.URL.Path != "" {
			return req.URL.Path, nil
		}
		return "/", nil

	case "@query":
		if req.URL == nil || req.URL.RawQuery == "" {
			return "?", nil
		}
		return "?" + req.URL.RawQuery, nil

	case "@authority":
		if req.Host != "" {
			return strings.ToLower(req.Host), nil
		}
		if req.URL != nil && req.URL.Host != "" {
			return strings.ToLower(req.URL.Host), nil
		}
		return "", fmt.Errorf("%w: @authority", ErrMissingComponent)

	case "@scheme":
		if req.URL != nil && req.URL.Scheme != "" {
			return strings.ToLower(req.URL.Scheme), nil
		}
		if req.TLS != nil {
			return "https", nil
		}
		return "http", nil

	case "@target-uri":
		if req.URL == nil || req.URL.String() == "" {
			return "", fmt.Errorf("%w: @target-uri", ErrMissingComponent)
		}
		return req.URL.String(), nil

	case "@request-target":
		if req.URL == nil {
			return "/", nil
		}
		target := req.URL.RequestURI()
		if target == "" {
			target = "/"
		}
		return target, nil

	default:
		if strings.HasPrefix(lower, "@") {
			return "", fmt.Errorf("%w: unknown derived component %s", ErrMissingComponent, component)
		}

		// Retrieve headers from http.Header (RFC 9421 Section 2.1: lowercase field name, combined by comma space)
		vals, ok := req.Header[http.CanonicalHeaderKey(component)]
		if !ok || len(vals) == 0 {
			if lower == "host" && req.Host != "" {
				return req.Host, nil
			}
			return "", fmt.Errorf("%w: header %q", ErrMissingComponent, component)
		}

		var trimmedVals []string
		for _, v := range vals {
			trimmedVals = append(trimmedVals, strings.TrimSpace(v))
		}
		return strings.Join(trimmedVals, ", "), nil
	}
}
