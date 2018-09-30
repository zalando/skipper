package auth

import (
	"net/http"
	"reflect"
	"testing"

	"github.com/zalando/skipper/routing"
)

func Test_spec_Create(t *testing.T) {
	tests := []struct {
		name    string
		typ     roleMatchType
		args    []interface{}
		want    routing.Predicate
		wantErr bool
	}{{
		name:    "invalid number of args",
		typ:     matchJWTPayloadAllKV,
		args:    []interface{}{"foo"},
		want:    nil,
		wantErr: true,
	}, {
		name: "one valid kv pair of args",
		typ:  matchJWTPayloadAllKV,
		args: []interface{}{"uid", "sszuecs"},
		want: &predicateAll{
			kv: map[string]string{
				"uid": "sszuecs",
			},
		},
		wantErr: false,
	}, {
		name: "one valid kv pair of args",
		typ:  matchJWTPayloadAnyKV,
		args: []interface{}{"uid", "sszuecs"},
		want: &predicateAny{
			kv: map[string]string{
				"uid": "sszuecs",
			},
		},
		wantErr: false,
	}, {
		name: "many valid kv pair of args",
		typ:  matchJWTPayloadAllKV,
		args: []interface{}{"uid", "sszuecs", "claim1", "claimValue1", "claim2", "claimValue2"},
		want: &predicateAll{
			kv: map[string]string{
				"uid":    "sszuecs",
				"claim1": "claimValue1",
				"claim2": "claimValue2",
			},
		},
		wantErr: false,
	}, {
		name: "many valid kv pair of args",
		typ:  matchJWTPayloadAnyKV,
		args: []interface{}{"uid", "sszuecs", "claim1", "claimValue1", "claim2", "claimValue2"},
		want: &predicateAny{
			kv: map[string]string{
				"uid":    "sszuecs",
				"claim1": "claimValue1",
				"claim2": "claimValue2",
			},
		},
		wantErr: false,
	}, {
		name:    "many kv pair of args, one missing",
		typ:     matchJWTPayloadAllKV,
		args:    []interface{}{"uid", "sszuecs", "claim1", "claimValue1", "claim2"},
		want:    nil,
		wantErr: true,
	}, {
		name:    "many kv pair of args",
		typ:     matchJWTPayloadAnyKV,
		args:    []interface{}{"uid", "sszuecs", "claim1", "claimValue1", "claim2"},
		want:    nil,
		wantErr: true,
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &spec{
				typ: tt.typ,
			}
			got, err := s.Create(tt.args)
			if (err != nil) != tt.wantErr {
				t.Errorf("spec.Create() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("spec.Create() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_predicateAll_Match(t *testing.T) {
	tests := []struct {
		name string
		kv   map[string]string
		tok  string
		want bool
	}{{
		name: "no valid kv pairs matching",
		kv: map[string]string{
			"https://identity.zalando.com/managed-id": "sszuecs",
			"https://identity.zalando.com/token":      "Bearer",
		},
		tok:  "eyJraWQiOiJwbGF0Zm9ybS1pYW0tdmNlaHloajYiLCJhbGciOiJFUzI1NiJ9.eyJzdWIiOiJjNGRkZmU5ZC1hMGQzLTRhZmItYmYyNi0yNGI5NTg4NzMxYTAiLCJodHRwczovL2lkZW50aXR5LnphbGFuZG8uY29tL3JlYWxtIjoidXNlcnMiLCJodHRwczovL2lkZW50aXR5LnphbGFuZG8uY29tL3Rva2VuIjoiQmVhcmVyIiwiaHR0cHM6Ly9pZGVudGl0eS56YWxhbmRvLmNvbS9tYW5hZ2VkLWlkIjoic3N6dWVjcyIsImF6cCI6Inp0b2tlbiIsImh0dHBzOi8vaWRlbnRpdHkuemFsYW5kby5jb20vYnAiOiI4MTBkMWQwMC00MzEyLTQzZTUtYmQzMS1kODM3M2ZkZDI0YzciLCJhdXRoX3RpbWUiOjE1MjMyNTk0NjgsImlzcyI6Imh0dHBzOi8vaWRlbnRpdHkuemFsYW5kby5jb20iLCJleHAiOjE1MjUwMjQyODUsImlhdCI6MTUyNTAyMDY3NX0.uxHcC7DJrkP-_G81Jmiba5liVP0LJOmkpal4wsUr7CmtMlE23P1bptIMxnJLv5EMSN1NFn-BJe9hcEB2A3LarA",
		want: true,
	}, {
		name: "many valid kv pairs matching",
		kv: map[string]string{
			"https://identity.zalando.com/managed-id": "sszuecs",
			"https://identity.zalando.com/token":      "Bearer",
		},
		tok:  "eyJraWQiOiJwbGF0Zm9ybS1pYW0tdmNlaHloajYiLCJhbGciOiJFUzI1NiJ9.eyJzdWIiOiJjNGRkZmU5ZC1hMGQzLTRhZmItYmYyNi0yNGI5NTg4NzMxYTAiLCJodHRwczovL2lkZW50aXR5LnphbGFuZG8uY29tL3JlYWxtIjoidXNlcnMiLCJodHRwczovL2lkZW50aXR5LnphbGFuZG8uY29tL3Rva2VuIjoiQmVhcmVyIiwiaHR0cHM6Ly9pZGVudGl0eS56YWxhbmRvLmNvbS9tYW5hZ2VkLWlkIjoic3N6dWVjcyIsImF6cCI6Inp0b2tlbiIsImh0dHBzOi8vaWRlbnRpdHkuemFsYW5kby5jb20vYnAiOiI4MTBkMWQwMC00MzEyLTQzZTUtYmQzMS1kODM3M2ZkZDI0YzciLCJhdXRoX3RpbWUiOjE1MjMyNTk0NjgsImlzcyI6Imh0dHBzOi8vaWRlbnRpdHkuemFsYW5kby5jb20iLCJleHAiOjE1MjUwMjQyODUsImlhdCI6MTUyNTAyMDY3NX0.uxHcC7DJrkP-_G81Jmiba5liVP0LJOmkpal4wsUr7CmtMlE23P1bptIMxnJLv5EMSN1NFn-BJe9hcEB2A3LarA",
		want: true,
	}, {
		name: "many valid kv pairs invalid token content",
		kv: map[string]string{
			"https://identity.zalando.com/managed-id": "sszuecs",
			"https://identity.zalando.com/token":      "Bearer",
		},
		tok:  "eyJraWQiOiJwbGF0Zm9ybS1pYW0tdmNlaHloajYiLCJhbGciOiJFUzI1NiJ9.31JzdWIiOiJjNG34ZmU5ZC1hMGQzLTRhZmItYmYyNi0yNGI5NTg4NzMxYTAiLCJodHRwczovL2lkZW50aXR5LnphbGFuZG8uY29tL3JlYWxtIjoidXNlcnMiLCJodHRwczovL2lkZW50aXR5LnphbGFuZG8uY29tL3Rva2VuIjoiQmVhcmVyIiwiaHR0cHM6Ly9pZGVudGl0eS56YWxhbmRvLmNvbS9tYW5hZ2VkLWlkIjoic3N6dWVjcyIsImF6cCI6Inp0b2tlbiIsImh0dHBzOi8vaWRlbnRpdHkuemFsYW5kby5jb20vYnAiOiI4MTBkMWQwMC00MzEyLTQzZTUtYmQzMS1kODM3M2ZkZDI0YzciLCJhdXRoX3RpbWUiOjE1MjMyNTk0NjgsImlzcyI6Imh0dHBzOi8vaWRlbnRpdHkuemFsYW5kby5jb20iLCJleHAiOjE1MjUwMjQyODUsImlhdCI6MTUyNTAyMDY3NX0.uxHcC7DJrkP-_G81Jmiba5liVP0LJOmkpal4wsUr7CmtMlE23P1bptIMxnJLv5EMSN1NFn-BJe9hcEB2A3LarA",
		want: false,
	}, {
		name: "many valid kv pairs invalid managed-id in token",
		kv: map[string]string{
			"https://identity.zalando.com/managed-id": "sszuecs",
			"https://identity.zalando.com/token":      "Bearer",
		},
		tok:  "eyJraWQiOiJwbGF0Zm9ybS1pYW0tdmNlaHloajYiLCJhbGciOiJFUzI1NiJ9.eyJzdWIiOiJjNGRkZmU5ZC1hMGQzLTRhZmItYmYyNi0yNGI5NTg4NzMxYTAiLCJodHRwczovL2lkZW50aXR5LnphbGFuZG8uY29tL3JlYWxtIjoidXNlcnMiLCJodHRwczovL2lkZW50aXR5LnphbGFuZG8uY29tL3Rva2VuIjoiQmVhcmVyIiwiaHR0cHM6Ly9pZGVudGl0eS56YWxhbmRvLmNvbS9tYW5hZ2VkLWlkIjoic29tZW9uZSIsImF6cCI6Inp0b2tlbiIsImh0dHBzOi8vaWRlbnRpdHkuemFsYW5kby5jb20vYnAiOiI4MTBkMWQwMC00MzEyLTQzZTUtYmQzMS1kODM3M2ZkZDI0YzciLCJhdXRoX3RpbWUiOjE1MjMyNTk0NjgsImlzcyI6Imh0dHBzOi8vaWRlbnRpdHkuemFsYW5kby5jb20iLCJleHAiOjE1MjUwMjQyODUsImlhdCI6MTUyNTAyMDY3NX0K.uxHcC7DJrkP-_G81Jmiba5liVP0LJOmkpal4wsUr7CmtMlE23P1bptIMxnJLv5EMSN1NFn-BJe9hcEB2A3LarA",
		want: false,
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &http.Request{
				Header: http.Header{
					authHeaderName: []string{"Bearer " + tt.tok},
				},
			}
			p := &predicateAll{
				kv: tt.kv,
			}
			if got := p.Match(r); got != tt.want {
				t.Errorf("predicateAll.Match() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_predicateAny_Match(t *testing.T) {
	tests := []struct {
		name string
		kv   map[string]string
		tok  string
		want bool
	}{{
		name: "no valid kv pairs matching",
		kv: map[string]string{
			"https://identity.zalando.com/managed-id": "sszuecs",
			"https://identity.zalando.com/token":      "Bearer",
		},
		tok:  "eyJraWQiOiJwbGF0Zm9ybS1pYW0tdmNlaHloajYiLCJhbGciOiJFUzI1NiJ9.eyJzdWIiOiJjNGRkZmU5ZC1hMGQzLTRhZmItYmYyNi0yNGI5NTg4NzMxYTAiLCJodHRwczovL2lkZW50aXR5LnphbGFuZG8uY29tL3JlYWxtIjoidXNlcnMiLCJodHRwczovL2lkZW50aXR5LnphbGFuZG8uY29tL3Rva2VuIjoiQmVhcmVyIiwiaHR0cHM6Ly9pZGVudGl0eS56YWxhbmRvLmNvbS9tYW5hZ2VkLWlkIjoic3N6dWVjcyIsImF6cCI6Inp0b2tlbiIsImh0dHBzOi8vaWRlbnRpdHkuemFsYW5kby5jb20vYnAiOiI4MTBkMWQwMC00MzEyLTQzZTUtYmQzMS1kODM3M2ZkZDI0YzciLCJhdXRoX3RpbWUiOjE1MjMyNTk0NjgsImlzcyI6Imh0dHBzOi8vaWRlbnRpdHkuemFsYW5kby5jb20iLCJleHAiOjE1MjUwMjQyODUsImlhdCI6MTUyNTAyMDY3NX0.uxHcC7DJrkP-_G81Jmiba5liVP0LJOmkpal4wsUr7CmtMlE23P1bptIMxnJLv5EMSN1NFn-BJe9hcEB2A3LarA",
		want: true,
	}, {
		name: "many valid kv pairs matching",
		kv: map[string]string{
			"https://identity.zalando.com/managed-id": "sszuecs",
			"https://identity.zalando.com/token":      "Bearer",
		},
		tok:  "eyJraWQiOiJwbGF0Zm9ybS1pYW0tdmNlaHloajYiLCJhbGciOiJFUzI1NiJ9.eyJzdWIiOiJjNGRkZmU5ZC1hMGQzLTRhZmItYmYyNi0yNGI5NTg4NzMxYTAiLCJodHRwczovL2lkZW50aXR5LnphbGFuZG8uY29tL3JlYWxtIjoidXNlcnMiLCJodHRwczovL2lkZW50aXR5LnphbGFuZG8uY29tL3Rva2VuIjoiQmVhcmVyIiwiaHR0cHM6Ly9pZGVudGl0eS56YWxhbmRvLmNvbS9tYW5hZ2VkLWlkIjoic3N6dWVjcyIsImF6cCI6Inp0b2tlbiIsImh0dHBzOi8vaWRlbnRpdHkuemFsYW5kby5jb20vYnAiOiI4MTBkMWQwMC00MzEyLTQzZTUtYmQzMS1kODM3M2ZkZDI0YzciLCJhdXRoX3RpbWUiOjE1MjMyNTk0NjgsImlzcyI6Imh0dHBzOi8vaWRlbnRpdHkuemFsYW5kby5jb20iLCJleHAiOjE1MjUwMjQyODUsImlhdCI6MTUyNTAyMDY3NX0.uxHcC7DJrkP-_G81Jmiba5liVP0LJOmkpal4wsUr7CmtMlE23P1bptIMxnJLv5EMSN1NFn-BJe9hcEB2A3LarA",
		want: true,
	}, {
		name: "many valid kv pairs invalid token content",
		kv: map[string]string{
			"https://identity.zalando.com/managed-id": "sszuecs",
			"https://identity.zalando.com/token":      "Bearer",
		},
		tok:  "eyJraWQiOiJwbGF0Zm9ybS1pYW0tdmNlaHloajYiLCJhbGciOiJFUzI1NiJ9.31JzdWIiOiJjNG34ZmU5ZC1hMGQzLTRhZmItYmYyNi0yNGI5NTg4NzMxYTAiLCJodHRwczovL2lkZW50aXR5LnphbGFuZG8uY29tL3JlYWxtIjoidXNlcnMiLCJodHRwczovL2lkZW50aXR5LnphbGFuZG8uY29tL3Rva2VuIjoiQmVhcmVyIiwiaHR0cHM6Ly9pZGVudGl0eS56YWxhbmRvLmNvbS9tYW5hZ2VkLWlkIjoic3N6dWVjcyIsImF6cCI6Inp0b2tlbiIsImh0dHBzOi8vaWRlbnRpdHkuemFsYW5kby5jb20vYnAiOiI4MTBkMWQwMC00MzEyLTQzZTUtYmQzMS1kODM3M2ZkZDI0YzciLCJhdXRoX3RpbWUiOjE1MjMyNTk0NjgsImlzcyI6Imh0dHBzOi8vaWRlbnRpdHkuemFsYW5kby5jb20iLCJleHAiOjE1MjUwMjQyODUsImlhdCI6MTUyNTAyMDY3NX0.uxHcC7DJrkP-_G81Jmiba5liVP0LJOmkpal4wsUr7CmtMlE23P1bptIMxnJLv5EMSN1NFn-BJe9hcEB2A3LarA",
		want: false,
	}, {
		name: "many valid kv pairs invalid managed-id in token",
		kv: map[string]string{
			"https://identity.zalando.com/managed-id": "sszuecs",
			"https://identity.zalando.com/token":      "Bearer",
		},
		tok:  "eyJraWQiOiJwbGF0Zm9ybS1pYW0tdmNlaHloajYiLCJhbGciOiJFUzI1NiJ9.eyJzdWIiOiJjNGRkZmU5ZC1hMGQzLTRhZmItYmYyNi0yNGI5NTg4NzMxYTAiLCJodHRwczovL2lkZW50aXR5LnphbGFuZG8uY29tL3JlYWxtIjoidXNlcnMiLCJodHRwczovL2lkZW50aXR5LnphbGFuZG8uY29tL3Rva2VuIjoiQmVhcmVyIiwiaHR0cHM6Ly9pZGVudGl0eS56YWxhbmRvLmNvbS9tYW5hZ2VkLWlkIjoic29tZW9uZSIsImF6cCI6Inp0b2tlbiIsImh0dHBzOi8vaWRlbnRpdHkuemFsYW5kby5jb20vYnAiOiI4MTBkMWQwMC00MzEyLTQzZTUtYmQzMS1kODM3M2ZkZDI0YzciLCJhdXRoX3RpbWUiOjE1MjMyNTk0NjgsImlzcyI6Imh0dHBzOi8vaWRlbnRpdHkuemFsYW5kby5jb20iLCJleHAiOjE1MjUwMjQyODUsImlhdCI6MTUyNTAyMDY3NX0K.uxHcC7DJrkP-_G81Jmiba5liVP0LJOmkpal4wsUr7CmtMlE23P1bptIMxnJLv5EMSN1NFn-BJe9hcEB2A3LarA",
		want: true,
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &http.Request{
				Header: http.Header{
					authHeaderName: []string{"Bearer " + tt.tok},
				},
			}
			p := &predicateAny{
				kv: tt.kv,
			}
			if got := p.Match(r); got != tt.want {
				t.Errorf("predicateAny.Match() = %v, want %v", got, tt.want)
			}
		})
	}
}
