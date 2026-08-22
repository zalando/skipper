package rfc9421

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// ErrMissingComponent is returned when a required component is missing from the HTTP request.
var ErrMissingComponent = errors.New("rfc9421: missing required component")

// BuildSignatureBase constructs the signature base string per RFC 9421 Section 2.5.
func (s *Signer) BuildSignatureBase(req *http.Request, sigInputParams string) (string, error) {
	var b strings.Builder

	for _, comp := range s.Components {
		val, err := s.getComponentValue(req, comp)
		if err != nil {
			return "", err
		}
		b.WriteString("\"")
		b.WriteString(strings.ToLower(strings.TrimSpace(comp)))
		b.WriteString("\": ")
		b.WriteString(val)
		b.WriteString("\n")
	}

	b.WriteString("\"@signature-params\": ")
	b.WriteString(sigInputParams)

	return b.String(), nil
}

func (s *Signer) getComponentValue(req *http.Request, component string) (string, error) {
	comp := strings.ToLower(strings.TrimSpace(component))

	if strings.HasPrefix(comp, "@") {
		return s.getDerivedComponent(req, comp)
	}

	return s.getHeaderComponent(req, comp)
}

func (s *Signer) getDerivedComponent(req *http.Request, component string) (string, error) {
	switch component {
	case "@method":
		method := req.Method
		if method == "" {
			method = http.MethodGet
		}
		return strings.ToUpper(method), nil

	case "@target-uri":
		if req.URL == nil {
			return "", fmt.Errorf("%w: URL is nil for @target-uri", ErrMissingComponent)
		}
		return req.URL.String(), nil

	case "@authority":
		host := req.Host
		if host == "" && req.URL != nil {
			host = req.URL.Host
		}
		if host == "" {
			return "", fmt.Errorf("%w: host is empty for @authority", ErrMissingComponent)
		}
		return strings.ToLower(host), nil

	case "@scheme":
		scheme := ""
		if req.URL != nil {
			scheme = req.URL.Scheme
		}
		if scheme == "" && req.TLS != nil {
			scheme = "https"
		}
		if scheme == "" {
			scheme = "http"
		}
		return strings.ToLower(scheme), nil

	case "@request-target":
		return s.getRequestTarget(req)

	case "@path":
		if req.URL == nil || req.URL.Path == "" {
			return "/", nil
		}
		return req.URL.Path, nil

	case "@query":
		if req.URL == nil || req.URL.RawQuery == "" {
			return "?", nil
		}
		return "?" + req.URL.RawQuery, nil

	default:
		return "", fmt.Errorf("%w: unsupported derived component %q", ErrMissingComponent, component)
	}
}

func (s *Signer) getRequestTarget(req *http.Request) (string, error) {
	if req.URL == nil {
		return "", fmt.Errorf("%w: URL is nil for @request-target", ErrMissingComponent)
	}
	path := req.URL.Path
	if path == "" {
		path = "/"
	}
	if req.URL.RawQuery != "" {
		path = path + "?" + req.URL.RawQuery
	}
	method := strings.ToLower(req.Method)
	if method == "" {
		method = "get"
	}
	return method + " " + path, nil
}

func (s *Signer) getHeaderComponent(req *http.Request, headerName string) (string, error) {
	if headerName == "host" {
		if req.Host != "" {
			return req.Host, nil
		}
		if req.URL != nil && req.URL.Host != "" {
			return req.URL.Host, nil
		}
		return "", fmt.Errorf("%w: header %q is missing", ErrMissingComponent, headerName)
	}

	values, ok := req.Header[http.CanonicalHeaderKey(headerName)]
	if !ok || len(values) == 0 {
		return "", fmt.Errorf("%w: header %q is missing", ErrMissingComponent, headerName)
	}

	var trimmed []string
	for _, v := range values {
		trimmed = append(trimmed, strings.TrimSpace(v))
	}
	return strings.Join(trimmed, ", "), nil
}

// CanonicalizeQuery normalizes query parameters if needed.
func CanonicalizeQuery(rawQuery string) (string, error) {
	if rawQuery == "" {
		return "", nil
	}
	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		return "", err
	}
	return values.Encode(), nil
}
