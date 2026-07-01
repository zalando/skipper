package builtin

import (
	"encoding/base64"
	"net/http"
	"testing"

	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/filtertest"
)

func TestEncodeBase64CreateFilter(t *testing.T) {
	tests := []struct {
		name    string
		newSpec func() filters.Spec
		args    []interface{}
		wantErr bool
	}{
		{
			name:    "valid: request header, no index",
			newSpec: NewEncodeRequestHeaderBase64,
			args:    []interface{}{"X-Custom"},
			wantErr: false,
		},
		{
			name:    "valid: request header, with index",
			newSpec: NewEncodeRequestHeaderBase64,
			args:    []interface{}{"X-Custom", float64(1)},
			wantErr: false,
		},
		{
			name:    "valid: response header, no index",
			newSpec: NewEncodeResponseHeaderBase64,
			args:    []interface{}{"X-Custom"},
			wantErr: false,
		},
		{
			name:    "valid: response header, with index",
			newSpec: NewEncodeResponseHeaderBase64,
			args:    []interface{}{"X-Custom", float64(0)},
			wantErr: false,
		},
		{
			name:    "error: no arguments",
			newSpec: NewEncodeRequestHeaderBase64,
			args:    []interface{}{},
			wantErr: true,
		},
		{
			name:    "error: too many arguments",
			newSpec: NewEncodeRequestHeaderBase64,
			args:    []interface{}{"X-Custom", float64(1), "extra"},
			wantErr: true,
		},
		{
			name:    "error: first arg not string",
			newSpec: NewEncodeRequestHeaderBase64,
			args:    []interface{}{42},
			wantErr: true,
		},
		{
			name:    "error: second arg not number",
			newSpec: NewEncodeRequestHeaderBase64,
			args:    []interface{}{"X-Custom", "not-a-number"},
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

func TestEncodeBase64RequestHeaderFullValue(t *testing.T) {
	tests := []struct {
		name        string
		headerName  string
		headerValue string
		expected    string
	}{
		{
			name:        "encode full value",
			headerName:  "X-Custom",
			headerValue: "hello",
			expected:    base64.StdEncoding.EncodeToString([]byte("hello")),
		},
		{
			name:        "empty header",
			headerName:  "X-Custom",
			headerValue: "",
			expected:    "",
		},
		{
			name:        "special characters",
			headerName:  "Authorization",
			headerValue: "user:pass123!@#",
			expected:    base64.StdEncoding.EncodeToString([]byte("user:pass123!@#")),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := NewEncodeRequestHeaderBase64()
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

func TestEncodeBase64RequestHeaderWithIndex(t *testing.T) {
	tests := []struct {
		name        string
		headerName  string
		headerValue string
		partIndex   float64
		expected    string
	}{
		{
			name:        "encode second part (index 1)",
			headerName:  "Authorization",
			headerValue: "Bearer token123",
			partIndex:   1,
			expected:    "Bearer " + base64.StdEncoding.EncodeToString([]byte("token123")),
		},
		{
			name:        "encode first part (index 0)",
			headerName:  "X-Custom",
			headerValue: "first second third",
			partIndex:   0,
			expected:    base64.StdEncoding.EncodeToString([]byte("first")) + " second third",
		},
		{
			name:        "index out of range",
			headerName:  "X-Custom",
			headerValue: "a b c",
			partIndex:   5,
			expected:    "a b c", // Original value remains on error
		},
		{
			name:        "negative index",
			headerName:  "X-Custom",
			headerValue: "a b c",
			partIndex:   -1,
			expected:    "a b c", // Original value remains on error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := NewEncodeRequestHeaderBase64()
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

func TestEncodeBase64ResponseHeaderFullValue(t *testing.T) {
	tests := []struct {
		name        string
		headerName  string
		headerValue string
		expected    string
	}{
		{
			name:        "encode full value",
			headerName:  "X-Custom",
			headerValue: "response-data",
			expected:    base64.StdEncoding.EncodeToString([]byte("response-data")),
		},
		{
			name:        "empty header",
			headerName:  "X-Custom",
			headerValue: "",
			expected:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := NewEncodeResponseHeaderBase64()
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

func TestEncodeBase64ResponseHeaderWithIndex(t *testing.T) {
	tests := []struct {
		name        string
		headerName  string
		headerValue string
		partIndex   float64
		expected    string
	}{
		{
			name:        "encode second part (index 1)",
			headerName:  "X-Custom",
			headerValue: "Header data",
			partIndex:   1,
			expected:    "Header " + base64.StdEncoding.EncodeToString([]byte("data")),
		},
		{
			name:        "index out of range",
			headerName:  "X-Custom",
			headerValue: "single",
			partIndex:   2,
			expected:    "single", // Original value remains on error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := NewEncodeResponseHeaderBase64()
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

func TestEncodeBase64Value(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		partIndex *int
		expected  string
		wantErr   bool
	}{
		{
			name:      "encode full value",
			value:     "hello",
			partIndex: nil,
			expected:  base64.StdEncoding.EncodeToString([]byte("hello")),
			wantErr:   false,
		},
		{
			name:      "encode with index 0",
			value:     "first second",
			partIndex: func() *int { i := 0; return &i }(),
			expected:  base64.StdEncoding.EncodeToString([]byte("first")) + " second",
			wantErr:   false,
		},
		{
			name:      "encode with index 1",
			value:     "first second",
			partIndex: func() *int { i := 1; return &i }(),
			expected:  "first " + base64.StdEncoding.EncodeToString([]byte("second")),
			wantErr:   false,
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
			result, err := encodeBase64Value(tt.value, tt.partIndex)
			if (err != nil) != tt.wantErr {
				t.Errorf("encodeBase64Value() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && result != tt.expected {
				t.Errorf("got %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestEncodeBase64FilterNames(t *testing.T) {
	tests := []struct {
		name     string
		newSpec  func() filters.Spec
		expected string
	}{
		{
			name:     "request header filter name",
			newSpec:  NewEncodeRequestHeaderBase64,
			expected: filters.EncodeBase64RequestHeaderName,
		},
		{
			name:     "response header filter name",
			newSpec:  NewEncodeResponseHeaderBase64,
			expected: filters.EncodeBase64ResponseHeaderName,
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

func TestEncodeBase64RealWorldScenario(t *testing.T) {
	// Scenario: Encode the second part of an Authorization header to base64
	spec := NewEncodeRequestHeaderBase64()
	f, err := spec.CreateFilter([]interface{}{"Authorization", float64(1)})
	if err != nil {
		t.Fatalf("CreateFilter failed: %v", err)
	}

	authHeader := "Bearer my-secret-token"

	req, err := http.NewRequest("GET", "https://example.org/api/v1/resource", nil)
	if err != nil {
		t.Fatalf("NewRequest failed: %v", err)
	}
	req.Header.Set("Authorization", authHeader)

	ctx := &filtertest.Context{FRequest: req}
	f.Request(ctx)

	result := req.Header.Get("Authorization")
	expected := "Bearer " + base64.StdEncoding.EncodeToString([]byte("my-secret-token"))
	if result != expected {
		t.Errorf("got %q, want %q", result, expected)
	}
}
