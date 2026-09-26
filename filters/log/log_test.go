package log

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/filtertest"
)

func TestRequest(t *testing.T) {
	for _, ti := range []struct {
		msg      string
		tok      string
		expected string
		input    []any
	}{
		{
			msg:      "request with token",
			tok:      "eyJraWQiOiJwbGF0Zm9ybS1pYW0tdmNlaHloajYiLCJhbGciOiJFUzI1NiJ9.eyJzdWIiOiJjNGRkZmU5ZC1hMGQzLTRhZmItYmYyNi0yNGI5NTg4NzMxYTAiLCJodHRwczovL2lkZW50aXR5LnphbGFuZG8uY29tL3JlYWxtIjoidXNlcnMiLCJodHRwczovL2lkZW50aXR5LnphbGFuZG8uY29tL3Rva2VuIjoiQmVhcmVyIiwiaHR0cHM6Ly9pZGVudGl0eS56YWxhbmRvLmNvbS9tYW5hZ2VkLWlkIjoic3N6dWVjcyIsImF6cCI6Inp0b2tlbiIsImh0dHBzOi8vaWRlbnRpdHkuemFsYW5kby5jb20vYnAiOiI4MTBkMWQwMC00MzEyLTQzZTUtYmQzMS1kODM3M2ZkZDI0YzciLCJhdXRoX3RpbWUiOjE1MjMyNTk0NjgsImlzcyI6Imh0dHBzOi8vaWRlbnRpdHkuemFsYW5kby5jb20iLCJleHAiOjE1MjUwMjQyODUsImlhdCI6MTUyNTAyMDY3NX0.uxHcC7DJrkP-_G81Jmiba5liVP0LJOmkpal4wsUr7CmtMlE23P1bptIMxnJLv5EMSN1NFn-BJe9hcEB2A3LarA",
			expected: "c4ddfe9d-a0d3-4afb-bf26-24b9588731a0",
			input:    []any{},
		},
		{
			msg:      "request with empty token",
			tok:      "",
			expected: "",
			input:    []any{},
		},
		{
			msg:      "request with wrong token",
			tok:      "foo.bar.baz",
			expected: "",
			input:    []any{},
		},
		{
			msg:      "request with prepared Sub in token, which does not contain valid data",
			tok:      "eyJraWQiOiJwbGF0Zm9ybS1pYW0tdmNlaHloajYiLCJhbGciOiJFUzI1NiJ9.eyJzdWIiOiIweK3e774iLCJodHRwczovL2lkZW50aXR5LnphbGFuZG8uY29tL3JlYWxtIjoidXNlcnMiLCJodHRwczovL2lkZW50aXR5LnphbGFuZG8uY29tL3Rva2VuIjoiQmVhcmVyIiwiaHR0cHM6Ly9pZGVudGl0eS56YWxhbmRvLmNvbS9tYW5hZ2VkLWlkIjoic3N6dWVjcyIsImF6cCI6Inp0b2tlbiIsImh0dHBzOi8vaWRlbnRpdHkuemFsYW5kby5jb20vYnAiOiI4MTBkMWQwMC00MzEyLTQzZTUtYmQzMS1kODM3M2ZkZDI0YzciLCJhdXRoX3RpbWUiOjE1MjMyNTk0NjgsImlzcyI6Imh0dHBzOi8vaWRlbnRpdHkuemFsYW5kby5jb20iLCJleHAiOjE1MjUwMjQyODUsImlhdCI6MTUyNTAyMDY3NX0K.uxHcC7DJrkP-_G81Jmiba5liVP0LJOmkpal4wsUr7CmtMlE23P1bptIMxnJLv5EMSN1NFn-BJe9hcEB2A3LarA",
			expected: defaultSub,
			input:    []any{},
		},
		{
			msg:      "request with prepared Sub in token, which does contain valid URI data",
			tok:      "eyJraWQiOiJwbGF0Zm9ybS1pYW0tdmNlaHloajYiLCJhbGciOiJFUzI1NiJ9.eyJzdWIiOiJodHRwOi8vZm9vLm9yZy9wMT9hPWIjNSIsImh0dHBzOi8vaWRlbnRpdHkuemFsYW5kby5jb20vcmVhbG0iOiJ1c2VycyIsImh0dHBzOi8vaWRlbnRpdHkuemFsYW5kby5jb20vdG9rZW4iOiJCZWFyZXIiLCJodHRwczovL2lkZW50aXR5LnphbGFuZG8uY29tL21hbmFnZWQtaWQiOiJzc3p1ZWNzIiwiYXpwIjoienRva2VuIiwiaHR0cHM6Ly9pZGVudGl0eS56YWxhbmRvLmNvbS9icCI6IjgxMGQxZDAwLTQzMTItNDNlNS1iZDMxLWQ4MzczZmRkMjRjNyIsImF1dGhfdGltZSI6MTUyMzI1OTQ2OCwiaXNzIjoiaHR0cHM6Ly9pZGVudGl0eS56YWxhbmRvLmNvbSIsImV4cCI6MTUyNTAyNDI4NSwiaWF0IjoxNTI1MDIwNjc1fQo.uxHcC7DJrkP-_G81Jmiba5liVP0LJOmkpal4wsUr7CmtMlE23P1bptIMxnJLv5EMSN1NFn-BJe9hcEB2A3LarA",
			expected: "http://foo.org/p1?a=b#5",
			input:    []any{},
		},
		{
			msg:      "request with sub passed as filter input",
			tok:      "eyJraWQiOiJwbGF0Zm9ybS1pYW0tdmNlaHloajYiLCJhbGciOiJFUzI1NiJ9.eyJzdWIiOiJjNGRkZmU5ZC1hMGQzLTRhZmItYmYyNi0yNGI5NTg4NzMxYTAiLCJodHRwczovL2lkZW50aXR5LnphbGFuZG8uY29tL3JlYWxtIjoidXNlcnMiLCJodHRwczovL2lkZW50aXR5LnphbGFuZG8uY29tL3Rva2VuIjoiQmVhcmVyIiwiaHR0cHM6Ly9pZGVudGl0eS56YWxhbmRvLmNvbS9tYW5hZ2VkLWlkIjoic3N6dWVjcyIsImF6cCI6Inp0b2tlbiIsImh0dHBzOi8vaWRlbnRpdHkuemFsYW5kby5jb20vYnAiOiI4MTBkMWQwMC00MzEyLTQzZTUtYmQzMS1kODM3M2ZkZDI0YzciLCJhdXRoX3RpbWUiOjE1MjMyNTk0NjgsImlzcyI6Imh0dHBzOi8vaWRlbnRpdHkuemFsYW5kby5jb20iLCJleHAiOjE1MjUwMjQyODUsImlhdCI6MTUyNTAyMDY3NX0.uxHcC7DJrkP-_G81Jmiba5liVP0LJOmkpal4wsUr7CmtMlE23P1bptIMxnJLv5EMSN1NFn-BJe9hcEB2A3LarA",
			expected: "c4ddfe9d-a0d3-4afb-bf26-24b9588731a0",
			input:    []any{"sub"},
		},
		{
			msg:      "request with foo and sub passed as filter input",
			tok:      "eyJraWQiOiJwbGF0Zm9ybS1pYW0tdmNlaHloajYiLCJhbGciOiJFUzI1NiJ9.eyJzdWIiOiJjNGRkZmU5ZC1hMGQzLTRhZmItYmYyNi0yNGI5NTg4NzMxYTAiLCJodHRwczovL2lkZW50aXR5LnphbGFuZG8uY29tL3JlYWxtIjoidXNlcnMiLCJodHRwczovL2lkZW50aXR5LnphbGFuZG8uY29tL3Rva2VuIjoiQmVhcmVyIiwiaHR0cHM6Ly9pZGVudGl0eS56YWxhbmRvLmNvbS9tYW5hZ2VkLWlkIjoic3N6dWVjcyIsImF6cCI6Inp0b2tlbiIsImh0dHBzOi8vaWRlbnRpdHkuemFsYW5kby5jb20vYnAiOiI4MTBkMWQwMC00MzEyLTQzZTUtYmQzMS1kODM3M2ZkZDI0YzciLCJhdXRoX3RpbWUiOjE1MjMyNTk0NjgsImlzcyI6Imh0dHBzOi8vaWRlbnRpdHkuemFsYW5kby5jb20iLCJleHAiOjE1MjUwMjQyODUsImlhdCI6MTUyNTAyMDY3NX0.uxHcC7DJrkP-_G81Jmiba5liVP0LJOmkpal4wsUr7CmtMlE23P1bptIMxnJLv5EMSN1NFn-BJe9hcEB2A3LarA",
			expected: "c4ddfe9d-a0d3-4afb-bf26-24b9588731a0",
			input:    []any{"foo", "sub"},
		},
		{
			msg:      "request with sub and bar passed as filter input",
			tok:      "eyJraWQiOiJwbGF0Zm9ybS1pYW0tdmNlaHloajYiLCJhbGciOiJFUzI1NiJ9.eyJzdWIiOiJjNGRkZmU5ZC1hMGQzLTRhZmItYmYyNi0yNGI5NTg4NzMxYTAiLCJodHRwczovL2lkZW50aXR5LnphbGFuZG8uY29tL3JlYWxtIjoidXNlcnMiLCJodHRwczovL2lkZW50aXR5LnphbGFuZG8uY29tL3Rva2VuIjoiQmVhcmVyIiwiaHR0cHM6Ly9pZGVudGl0eS56YWxhbmRvLmNvbS9tYW5hZ2VkLWlkIjoic3N6dWVjcyIsImF6cCI6Inp0b2tlbiIsImh0dHBzOi8vaWRlbnRpdHkuemFsYW5kby5jb20vYnAiOiI4MTBkMWQwMC00MzEyLTQzZTUtYmQzMS1kODM3M2ZkZDI0YzciLCJhdXRoX3RpbWUiOjE1MjMyNTk0NjgsImlzcyI6Imh0dHBzOi8vaWRlbnRpdHkuemFsYW5kby5jb20iLCJleHAiOjE1MjUwMjQyODUsImlhdCI6MTUyNTAyMDY3NX0.uxHcC7DJrkP-_G81Jmiba5liVP0LJOmkpal4wsUr7CmtMlE23P1bptIMxnJLv5EMSN1NFn-BJe9hcEB2A3LarA",
			expected: "c4ddfe9d-a0d3-4afb-bf26-24b9588731a0",
			input:    []any{"sub", "bar"},
		},
		{
			msg:      "request with non sub passed as filter input",
			tok:      "eyJraWQiOiJwbGF0Zm9ybS1pYW0tdmNlaHloajYiLCJhbGciOiJFUzI1NiJ9.eyJzdWIiOiJjNGRkZmU5ZC1hMGQzLTRhZmItYmYyNi0yNGI5NTg4NzMxYTAiLCJodHRwczovL2lkZW50aXR5LnphbGFuZG8uY29tL3JlYWxtIjoidXNlcnMiLCJodHRwczovL2lkZW50aXR5LnphbGFuZG8uY29tL3Rva2VuIjoiQmVhcmVyIiwiaHR0cHM6Ly9pZGVudGl0eS56YWxhbmRvLmNvbS9tYW5hZ2VkLWlkIjoic3N6dWVjcyIsImF6cCI6Inp0b2tlbiIsImh0dHBzOi8vaWRlbnRpdHkuemFsYW5kby5jb20vYnAiOiI4MTBkMWQwMC00MzEyLTQzZTUtYmQzMS1kODM3M2ZkZDI0YzciLCJhdXRoX3RpbWUiOjE1MjMyNTk0NjgsImlzcyI6Imh0dHBzOi8vaWRlbnRpdHkuemFsYW5kby5jb20iLCJleHAiOjE1MjUwMjQyODUsImlhdCI6MTUyNTAyMDY3NX0.uxHcC7DJrkP-_G81Jmiba5liVP0LJOmkpal4wsUr7CmtMlE23P1bptIMxnJLv5EMSN1NFn-BJe9hcEB2A3LarA",
			expected: "ztoken",
			input:    []any{"azp"},
		},
		{
			msg:      "request with invalid key passed as filter input",
			tok:      "eyJraWQiOiJwbGF0Zm9ybS1pYW0tdmNlaHloajYiLCJhbGciOiJFUzI1NiJ9.eyJzdWIiOiJjNGRkZmU5ZC1hMGQzLTRhZmItYmYyNi0yNGI5NTg4NzMxYTAiLCJodHRwczovL2lkZW50aXR5LnphbGFuZG8uY29tL3JlYWxtIjoidXNlcnMiLCJodHRwczovL2lkZW50aXR5LnphbGFuZG8uY29tL3Rva2VuIjoiQmVhcmVyIiwiaHR0cHM6Ly9pZGVudGl0eS56YWxhbmRvLmNvbS9tYW5hZ2VkLWlkIjoic3N6dWVjcyIsImF6cCI6Inp0b2tlbiIsImh0dHBzOi8vaWRlbnRpdHkuemFsYW5kby5jb20vYnAiOiI4MTBkMWQwMC00MzEyLTQzZTUtYmQzMS1kODM3M2ZkZDI0YzciLCJhdXRoX3RpbWUiOjE1MjMyNTk0NjgsImlzcyI6Imh0dHBzOi8vaWRlbnRpdHkuemFsYW5kby5jb20iLCJleHAiOjE1MjUwMjQyODUsImlhdCI6MTUyNTAyMDY3NX0.uxHcC7DJrkP-_G81Jmiba5liVP0LJOmkpal4wsUr7CmtMlE23P1bptIMxnJLv5EMSN1NFn-BJe9hcEB2A3LarA",
			expected: "",
			input:    []any{"blahBlah"},
		},
	} {
		t.Run(ti.msg, func(t *testing.T) {
			spec := &unverifiedAuditLogSpec{}

			fltr, err := spec.CreateFilter(ti.input)
			if err != nil {
				t.Errorf("Failed to create filter: %v", err)
				return
			}

			req, err := http.NewRequest("GET", "http://localhost/", nil)
			if err != nil {
				t.Errorf("Failed to create request: %v", err)
				return
			}

			ctx := &filtertest.Context{
				FStateBag: make(map[string]any),
				FRequest:  req,
			}
			ctx.FRequest.Header.Add(authHeaderName, authHeaderPrefix+ti.tok)

			fltr.Request(ctx)

			s := ctx.Request().Header.Get(UnverifiedAuditHeader)
			if s != ti.expected {
				t.Errorf("Unexpected result: '%s' != '%s'", s, ti.expected)
				return
			}
		})
	}
}

func TestUnverifiedAuditLogSpec(t *testing.T) {
	spec := NewUnverifiedAuditLog()
	if spec.Name() != filters.UnverifiedAuditLogName {
		t.Fatalf("expected name %s, got %s", filters.UnverifiedAuditLogName, spec.Name())
	}

	_, err := spec.CreateFilter([]any{123})
	if !errors.Is(err, filters.ErrInvalidFilterParameters) {
		t.Fatalf("expected ErrInvalidFilterParameters for non-string arg, got %v", err)
	}

	fltr, err := spec.CreateFilter(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	fltr.Response(&filtertest.Context{})
}

func TestAuditLogSpec(t *testing.T) {
	spec := NewAuditLog(1024)
	if spec.Name() != filters.AuditLogName {
		t.Fatalf("expected name %s, got %s", filters.AuditLogName, spec.Name())
	}

	_, err := spec.CreateFilter([]any{"unexpected"})
	if !errors.Is(err, filters.ErrInvalidFilterParameters) {
		t.Fatalf("expected ErrInvalidFilterParameters, got %v", err)
	}

	fltr, err := spec.CreateFilter(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fltr == nil {
		t.Fatal("expected non-nil filter")
	}
}

func TestTeeBody(t *testing.T) {
	t.Run("bounded maxTee", func(t *testing.T) {
		content := "hello world from teeBody"
		rc := io.NopCloser(strings.NewReader(content))
		tb := newTeeBody(rc, 5)

		buf, err := io.ReadAll(tb)
		if err != nil {
			t.Fatalf("unexpected read error: %v", err)
		}
		if string(buf) != content {
			t.Fatalf("expected read content %q, got %q", content, string(buf))
		}

		if err := tb.Close(); err != nil {
			t.Fatalf("unexpected close error: %v", err)
		}

		tBody := tb.(*teeBody)
		if tBody.buffer.String() != "hello" {
			t.Fatalf("expected buffer to be truncated to 'hello', got %q", tBody.buffer.String())
		}
	})

	t.Run("unbounded maxTee negative", func(t *testing.T) {
		content := "complete content without limit"
		rc := io.NopCloser(strings.NewReader(content))
		tb := newTeeBody(rc, -1)

		buf, err := io.ReadAll(tb)
		if err != nil {
			t.Fatalf("unexpected read error: %v", err)
		}
		if string(buf) != content {
			t.Fatalf("expected read content %q, got %q", content, string(buf))
		}

		tBody := tb.(*teeBody)
		if tBody.buffer.String() != content {
			t.Fatalf("expected buffer to match full content, got %q", tBody.buffer.String())
		}
	})
}

func TestAuditLogFilter(t *testing.T) {
	t.Run("records request body and auth status", func(t *testing.T) {
		out := &bytes.Buffer{}
		al := &auditLog{
			writer:     out,
			maxBodyLog: 1024,
		}

		req, err := http.NewRequest("POST", "http://localhost/api/test", io.NopCloser(strings.NewReader("payload-data")))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		ctx := &filtertest.Context{
			FStateBag: map[string]any{
				AuthUserKey:         "john_doe",
				AuthRejectReasonKey: "insufficient_scope",
			},
			FRequest:  req,
			FResponse: &http.Response{StatusCode: http.StatusForbidden},
		}

		al.Request(ctx)
		al.Response(ctx)

		var doc auditDoc
		if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
			t.Fatalf("failed to decode audit log JSON: %v, raw: %s", err, out.String())
		}

		if doc.Method != "POST" {
			t.Errorf("expected Method POST, got %s", doc.Method)
		}
		if doc.Path != "/api/test" {
			t.Errorf("expected Path /api/test, got %s", doc.Path)
		}
		if doc.Status != http.StatusForbidden {
			t.Errorf("expected Status 403, got %d", doc.Status)
		}
		if doc.RequestBody != "payload-data" {
			t.Errorf("expected RequestBody 'payload-data', got %q", doc.RequestBody)
		}
		if doc.AuthStatus == nil {
			t.Fatal("expected AuthStatus to be populated")
		}
		if doc.AuthStatus.User != "john_doe" {
			t.Errorf("expected user 'john_doe', got %s", doc.AuthStatus.User)
		}
		if !doc.AuthStatus.Rejected {
			t.Errorf("expected Rejected to be true")
		}
		if doc.AuthStatus.Reason != "insufficient_scope" {
			t.Errorf("expected Reason 'insufficient_scope', got %s", doc.AuthStatus.Reason)
		}
	})

	t.Run("zero maxBodyLog does not wrap body in teeBody", func(t *testing.T) {
		al := &auditLog{
			writer:     &bytes.Buffer{},
			maxBodyLog: 0,
		}

		req, _ := http.NewRequest("GET", "http://localhost/", io.NopCloser(strings.NewReader("test")))
		ctx := &filtertest.Context{
			FRequest:  req,
			FResponse: &http.Response{StatusCode: http.StatusOK},
		}

		al.Request(ctx)
		if _, ok := ctx.Request().Body.(*teeBody); ok {
			t.Fatal("expected body NOT to be wrapped in teeBody when maxBodyLog == 0")
		}
	})

	t.Run("negative maxBodyLog copies full body", func(t *testing.T) {
		out := &bytes.Buffer{}
		al := &auditLog{
			writer:     out,
			maxBodyLog: -1,
		}

		bodyContent := "unlimited-length-body"
		req, _ := http.NewRequest("PUT", "http://localhost/unlimited", io.NopCloser(strings.NewReader(bodyContent)))
		ctx := &filtertest.Context{
			FRequest:  req,
			FResponse: &http.Response{StatusCode: http.StatusOK},
		}

		al.Request(ctx)
		al.Response(ctx)

		var doc auditDoc
		if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
			t.Fatalf("failed to parse JSON: %v", err)
		}
		if doc.RequestBody != bodyContent {
			t.Errorf("expected body %q, got %q", bodyContent, doc.RequestBody)
		}
	})
}
