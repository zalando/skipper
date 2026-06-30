package builtin

import (
	"encoding/base64"
	"net/http"
	"testing"

	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/filtertest"
)

func TestDecodeBase64CreateFilter(t *testing.T) {
	tests := []struct {
		name    string
		newSpec func() filters.Spec
		args    []interface{}
		wantErr bool
	}{
		{
			name:    "valid: request header, no index",
			newSpec: NewDecodeRequestHeaderBase64,
			args:    []interface{}{"Authorization"},
			wantErr: false,
		},
		{
			name:    "valid: request header, with index",
			newSpec: NewDecodeRequestHeaderBase64,
			args:    []interface{}{"Authorization", float64(1)},
			wantErr: false,
		},
		{
			name:    "valid: response header, no index",
			newSpec: NewDecodeResponseHeaderBase64,
			args:    []interface{}{"X-Custom"},
			wantErr: false,
		},
		{
			name:    "valid: response header, with index",
			newSpec: NewDecodeResponseHeaderBase64,
			args:    []interface{}{"X-Custom", float64(0)},
			wantErr: false,
		},
		{
			name:    "error: no arguments",
			newSpec: NewDecodeRequestHeaderBase64,
			args:    []interface{}{},
			wantErr: true,
		},
		{
			name:    "error: too many arguments",
			newSpec: NewDecodeRequestHeaderBase64,
			args:    []interface{}{"Authorization", float64(1), "extra"},
			wantErr: true,
		},
		{
			name:    "error: first arg not string",
			newSpec: NewDecodeRequestHeaderBase64,
			args:    []interface{}{42},
			wantErr: true,
		},
		{
			name:    "error: second arg not number",
			newSpec: NewDecodeRequestHeaderBase64,
			args:    []interface{}{"Authorization", "not-a-number"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := tt.newSpec()
			f, err := spec.CreateFilter(tt.args)
			if (err != nil) != tt.wantErr {
				t.Errorf("CreateFilter() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && f == nil {
				t.Errorf("CreateFilter() returned nil filter")
			}
		})
	}
}

func TestDecodeBase64RequestHeaderFullValue(t *testing.T) {
	tests := []struct {
		name        string
		headerName  string
		headerValue string
		expected    string
		wantErr     bool
	}{
		{
			name:        "valid base64",
			headerName:  "Authorization",
			headerValue: base64.StdEncoding.EncodeToString([]byte("user:password")),
			expected:    "user:password",
			wantErr:     false,
		},
		{
			name:        "empty header",
			headerName:  "Authorization",
			headerValue: "",
			expected:    "",
			wantErr:     false,
		},
		{
			name:        "invalid base64 - should not crash",
			headerName:  "Authorization",
			headerValue: "invalid!!!base64",
			expected:    "invalid!!!base64", // Original value remains on error
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := NewDecodeRequestHeaderBase64()
			f, err := spec.CreateFilter([]interface{}{tt.headerName})
			if err != nil {
				t.Fatalf("CreateFilter failed: %v", err)
			}

			req, err := http.NewRequest("GET", "https://example.org/test", nil)
			if err != nil {
				t.Fatalf("NewRequest failed: %v", err)
			}

			if tt.headerValue != "" {
				req.Header.Set(tt.headerName, tt.headerValue)
			}

			ctx := &filtertest.Context{FRequest: req}
			f.Request(ctx)

			result := req.Header.Get(tt.headerName)
			if result != tt.expected {
				t.Errorf("got %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestDecodeBase64RequestHeaderWithIndex(t *testing.T) {
	tests := []struct {
		name        string
		headerName  string
		headerValue string
		partIndex   float64
		expected    string
		wantErr     bool
	}{
		{
			name:        "decode second part (index 1)",
			headerName:  "Authorization",
			headerValue: "Bearer " + base64.StdEncoding.EncodeToString([]byte("token123")),
			partIndex:   1,
			expected:    "Bearer token123",
			wantErr:     false,
		},
		{
			name:        "decode first part (index 0)",
			headerName:  "Authorization",
			headerValue: base64.StdEncoding.EncodeToString([]byte("Basic")) + " " + base64.StdEncoding.EncodeToString([]byte("credentials")),
			partIndex:   0,
			expected:    "Basic " + base64.StdEncoding.EncodeToString([]byte("credentials")),
			wantErr:     false,
		},
		{
			name:        "index out of range",
			headerName:  "Authorization",
			headerValue: "Bearer token",
			partIndex:   5,
			expected:    "Bearer token", // Original value remains on error
			wantErr:     false,
		},
		{
			name:        "negative index",
			headerName:  "Authorization",
			headerValue: "Bearer token",
			partIndex:   -1,
			expected:    "Bearer token", // Original value remains on error
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := NewDecodeRequestHeaderBase64()
			f, err := spec.CreateFilter([]interface{}{tt.headerName, tt.partIndex})
			if err != nil {
				t.Fatalf("CreateFilter failed: %v", err)
			}

			req, err := http.NewRequest("GET", "https://example.org/test", nil)
			if err != nil {
				t.Fatalf("NewRequest failed: %v", err)
			}

			req.Header.Set(tt.headerName, tt.headerValue)

			ctx := &filtertest.Context{FRequest: req}
			f.Request(ctx)

			result := req.Header.Get(tt.headerName)
			if result != tt.expected {
				t.Errorf("got %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestDecodeBase64ResponseHeaderFullValue(t *testing.T) {
	tests := []struct {
		name        string
		headerName  string
		headerValue string
		expected    string
		wantErr     bool
	}{
		{
			name:        "valid base64",
			headerName:  "X-Custom",
			headerValue: base64.StdEncoding.EncodeToString([]byte("response-data")),
			expected:    "response-data",
			wantErr:     false,
		},
		{
			name:        "empty header",
			headerName:  "X-Custom",
			headerValue: "",
			expected:    "",
			wantErr:     false,
		},
		{
			name:        "invalid base64",
			headerName:  "X-Custom",
			headerValue: "not-valid-base64!!!",
			expected:    "not-valid-base64!!!", // Original value remains on error
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := NewDecodeResponseHeaderBase64()
			f, err := spec.CreateFilter([]interface{}{tt.headerName})
			if err != nil {
				t.Fatalf("CreateFilter failed: %v", err)
			}

			resp := &http.Response{Header: make(http.Header)}
			if tt.headerValue != "" {
				resp.Header.Set(tt.headerName, tt.headerValue)
			}

			ctx := &filtertest.Context{FResponse: resp}
			f.Response(ctx)

			result := resp.Header.Get(tt.headerName)
			if result != tt.expected {
				t.Errorf("got %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestDecodeBase64ResponseHeaderWithIndex(t *testing.T) {
	tests := []struct {
		name        string
		headerName  string
		headerValue string
		partIndex   float64
		expected    string
		wantErr     bool
	}{
		{
			name:        "decode second part (index 1)",
			headerName:  "X-Custom",
			headerValue: "Header " + base64.StdEncoding.EncodeToString([]byte("data")),
			partIndex:   1,
			expected:    "Header data",
			wantErr:     false,
		},
		{
			name:        "index out of range",
			headerName:  "X-Custom",
			headerValue: "single",
			partIndex:   2,
			expected:    "single", // Original value remains on error
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := NewDecodeResponseHeaderBase64()
			f, err := spec.CreateFilter([]interface{}{tt.headerName, tt.partIndex})
			if err != nil {
				t.Fatalf("CreateFilter failed: %v", err)
			}

			resp := &http.Response{Header: make(http.Header)}
			resp.Header.Set(tt.headerName, tt.headerValue)

			ctx := &filtertest.Context{FResponse: resp}
			f.Response(ctx)

			result := resp.Header.Get(tt.headerName)
			if result != tt.expected {
				t.Errorf("got %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestDecodeBase64Value(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		partIndex *int
		expected  string
		wantErr   bool
	}{
		{
			name:      "decode full value",
			value:     base64.StdEncoding.EncodeToString([]byte("hello")),
			partIndex: nil,
			expected:  "hello",
			wantErr:   false,
		},
		{
			name:      "decode with index 0",
			value:     base64.StdEncoding.EncodeToString([]byte("first")) + " second",
			partIndex: func() *int { i := 0; return &i }(),
			expected:  "first second",
			wantErr:   false,
		},
		{
			name:      "decode with index 1",
			value:     "first " + base64.StdEncoding.EncodeToString([]byte("second")),
			partIndex: func() *int { i := 1; return &i }(),
			expected:  "first second",
			wantErr:   false,
		},
		{
			name:      "invalid base64",
			value:     "not-valid-base64!!!",
			partIndex: nil,
			expected:  "",
			wantErr:   true,
		},
		{
			name:      "invalid base64 in part",
			value:     "first not-valid!!!",
			partIndex: func() *int { i := 1; return &i }(),
			expected:  "",
			wantErr:   true,
		},
		{
			name:      "part index out of range",
			value:     "single",
			partIndex: func() *int { i := 5; return &i }(),
			expected:  "",
			wantErr:   true,
		},
		{
			name:      "part index negative",
			value:     "single",
			partIndex: func() *int { i := -1; return &i }(),
			expected:  "",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := decodeBase64Value(tt.value, tt.partIndex)
			if (err != nil) != tt.wantErr {
				t.Errorf("decodeBase64Value() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && result != tt.expected {
				t.Errorf("got %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestDecodeBase64FilterNames(t *testing.T) {
	tests := []struct {
		name     string
		newSpec  func() filters.Spec
		expected string
	}{
		{
			name:     "request header filter name",
			newSpec:  NewDecodeRequestHeaderBase64,
			expected: filters.DecodeBase64RequestHeaderName,
		},
		{
			name:     "response header filter name",
			newSpec:  NewDecodeResponseHeaderBase64,
			expected: filters.DecodeBase64ResponseHeaderName,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := tt.newSpec()
			if spec.Name() != tt.expected {
				t.Errorf("Name() = %q, want %q", spec.Name(), tt.expected)
			}
		})
	}
}

func TestDecodeBase64RealWorldScenario(t *testing.T) {
	// Scenario: Client sends base64 encoded credentials, server needs to decode them
	spec := NewDecodeRequestHeaderBase64()
	f, err := spec.CreateFilter([]interface{}{"Authorization", float64(1)})
	if err != nil {
		t.Fatalf("CreateFilter failed: %v", err)
	}

	// Simulate: "Bearer " + base64(user:password)
	credentials := base64.StdEncoding.EncodeToString([]byte("user:password"))
	authHeader := "Bearer " + credentials

	req, err := http.NewRequest("GET", "https://example.org/api/v1/resource", nil)
	if err != nil {
		t.Fatalf("NewRequest failed: %v", err)
	}
	req.Header.Set("Authorization", authHeader)

	ctx := &filtertest.Context{FRequest: req}
	f.Request(ctx)

	result := req.Header.Get("Authorization")
	expected := "Bearer user:password"
	if result != expected {
		t.Errorf("got %q, want %q", result, expected)
	}
}
