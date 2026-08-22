package rfc9421

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// errMissingComponent is returned when a required component is missing from the HTTP request.
var errMissingComponent = errors.New("rfc9421: missing required component")

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
		return s.getTargetURI(req)

	case "@authority":
		host := req.Host
		if host == "" && req.URL != nil {
			host = req.URL.Host
		}
		if host == "" {
			return "", fmt.Errorf("%w: host is empty for @authority", errMissingComponent)
		}
		host = stripDefaultPort(host, s.getScheme(req))
		return strings.ToLower(host), nil

	case "@scheme":
		scheme := s.getScheme(req)
		return strings.ToLower(scheme), nil

	case "@request-target":
		return s.getRequestTarget(req)

	case "@path":
		if req.URL == nil {
			return "/", nil
		}
		escapedPath := req.URL.EscapedPath()
		if escapedPath == "" {
			escapedPath = req.URL.Path
		}
		if escapedPath == "" {
			return "/", nil
		}
		if !strings.HasPrefix(escapedPath, "/") {
			escapedPath = "/" + escapedPath
		}
		return escapedPath, nil

	case "@query":
		if req.URL == nil || req.URL.RawQuery == "" {
			return "?", nil
		}
		return "?" + req.URL.RawQuery, nil

	default:
		return "", fmt.Errorf("%w: unsupported derived component %q", errMissingComponent, component)
	}
}

func (s *Signer) getScheme(req *http.Request) string {
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
	return scheme
}

func stripDefaultPort(host string, scheme string) string {
	lowerScheme := strings.ToLower(scheme)
	if lowerScheme == "http" && strings.HasSuffix(host, ":80") {
		return strings.TrimSuffix(host, ":80")
	}
	if lowerScheme == "https" && strings.HasSuffix(host, ":443") {
		return strings.TrimSuffix(host, ":443")
	}
	return host
}

func (s *Signer) getTargetURI(req *http.Request) (string, error) {
	if req.URL == nil {
		return "", fmt.Errorf("%w: URL is nil for @target-uri", errMissingComponent)
	}

	scheme := s.getScheme(req)

	host := req.Host
	if host == "" {
		host = req.URL.Host
	}
	if host == "" {
		return "", fmt.Errorf("%w: host is empty for @target-uri", errMissingComponent)
	}
	host = stripDefaultPort(host, scheme)

	path := req.URL.EscapedPath()
	if path == "" {
		path = req.URL.Path
	}
	if path == "" {
		path = "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if req.URL.RawQuery != "" {
		path = path + "?" + req.URL.RawQuery
	}

	return fmt.Sprintf("%s://%s%s", strings.ToLower(scheme), strings.ToLower(host), path), nil
}

func (s *Signer) getRequestTarget(req *http.Request) (string, error) {
	if req.URL == nil {
		return "", fmt.Errorf("%w: URL is nil for @request-target", errMissingComponent)
	}
	path := req.URL.EscapedPath()
	if path == "" {
		path = req.URL.Path
	}
	if path == "" {
		path = "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if req.URL.RawQuery != "" {
		path = path + "?" + req.URL.RawQuery
	}
	return path, nil
}

func (s *Signer) getHeaderComponent(req *http.Request, headerName string) (string, error) {
	if headerName == "host" {
		if req.Host != "" {
			return req.Host, nil
		}
		if req.URL != nil && req.URL.Host != "" {
			return req.URL.Host, nil
		}
		return "", fmt.Errorf("%w: header %q is missing", errMissingComponent, headerName)
	}

	values, ok := req.Header[http.CanonicalHeaderKey(headerName)]
	if !ok || len(values) == 0 {
		return "", fmt.Errorf("%w: header %q is missing", errMissingComponent, headerName)
	}

	var trimmed []string
	for _, v := range values {
		trimmed = append(trimmed, strings.TrimSpace(v))
	}
	return strings.Join(trimmed, ", "), nil
}
