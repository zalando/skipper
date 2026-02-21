package log

import (
	"net/http"
	"testing"

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
