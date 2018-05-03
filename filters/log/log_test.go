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
	}{
		{
			msg:      "request with token",
			tok:      "eyJraWQiOiJwbGF0Zm9ybS1pYW0tdmNlaHloajYiLCJhbGciOiJFUzI1NiJ9.eyJzdWIiOiJjNGRkZmU5ZC1hMGQzLTRhZmItYmYyNi0yNGI5NTg4NzMxYTAiLCJodHRwczovL2lkZW50aXR5LnphbGFuZG8uY29tL3JlYWxtIjoidXNlcnMiLCJodHRwczovL2lkZW50aXR5LnphbGFuZG8uY29tL3Rva2VuIjoiQmVhcmVyIiwiaHR0cHM6Ly9pZGVudGl0eS56YWxhbmRvLmNvbS9tYW5hZ2VkLWlkIjoic3N6dWVjcyIsImF6cCI6Inp0b2tlbiIsImh0dHBzOi8vaWRlbnRpdHkuemFsYW5kby5jb20vYnAiOiI4MTBkMWQwMC00MzEyLTQzZTUtYmQzMS1kODM3M2ZkZDI0YzciLCJhdXRoX3RpbWUiOjE1MjMyNTk0NjgsImlzcyI6Imh0dHBzOi8vaWRlbnRpdHkuemFsYW5kby5jb20iLCJleHAiOjE1MjUwMjQyODUsImlhdCI6MTUyNTAyMDY3NX0.uxHcC7DJrkP-_G81Jmiba5liVP0LJOmkpal4wsUr7CmtMlE23P1bptIMxnJLv5EMSN1NFn-BJe9hcEB2A3LarA",
			expected: "c4ddfe9d-a0d3-4afb-bf26-24b9588731a0",
		},
		{
			msg:      "request with empty token",
			tok:      "",
			expected: "",
		},
		{
			msg:      "request with wrong token",
			tok:      "foo.bar.baz",
			expected: "",
		},
		{
			msg:      "request with prepared Sub in token, which does not contain valid data",
			tok:      "eyJraWQiOiJwbGF0Zm9ybS1pYW0tdmNlaHloajYiLCJhbGciOiJFUzI1NiJ9.eyJzdWIiOiIweK3e774iLCJodHRwczovL2lkZW50aXR5LnphbGFuZG8uY29tL3JlYWxtIjoidXNlcnMiLCJodHRwczovL2lkZW50aXR5LnphbGFuZG8uY29tL3Rva2VuIjoiQmVhcmVyIiwiaHR0cHM6Ly9pZGVudGl0eS56YWxhbmRvLmNvbS9tYW5hZ2VkLWlkIjoic3N6dWVjcyIsImF6cCI6Inp0b2tlbiIsImh0dHBzOi8vaWRlbnRpdHkuemFsYW5kby5jb20vYnAiOiI4MTBkMWQwMC00MzEyLTQzZTUtYmQzMS1kODM3M2ZkZDI0YzciLCJhdXRoX3RpbWUiOjE1MjMyNTk0NjgsImlzcyI6Imh0dHBzOi8vaWRlbnRpdHkuemFsYW5kby5jb20iLCJleHAiOjE1MjUwMjQyODUsImlhdCI6MTUyNTAyMDY3NX0K.uxHcC7DJrkP-_G81Jmiba5liVP0LJOmkpal4wsUr7CmtMlE23P1bptIMxnJLv5EMSN1NFn-BJe9hcEB2A3LarA",
			expected: defaultSub,
		},
	} {
		t.Run(ti.msg, func(t *testing.T) {
			spec := &unverifiedAuditLog{}

			fltr, err := spec.CreateFilter([]interface{}{})
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
				FStateBag: make(map[string]interface{}),
				FRequest:  req,
			}
			ctx.FRequest.Header.Add(authHeaderName, authHeaderPrefix+ti.tok)

			fltr.Request(ctx)

			s := ctx.Request().Header.Get(UnverifiedAuditHeader)
			if s != ti.expected {
				t.Errorf("Unexpected result: %s != %s", s, ti.expected)
				return
			}

		})
	}
}
