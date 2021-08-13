package auth

import (
	"net/http"
	"reflect"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zalando/skipper/routing"
)

func Test_spec(t *testing.T) {
	for _, tc := range []struct {
		spec routing.PredicateSpec
		name string
	}{
		{
			spec: NewJWTPayloadAllKV(),
			name: MatchJWTPayloadAllKVName,
		},
		{
			spec: NewJWTPayloadAnyKV(),
			name: MatchJWTPayloadAnyKVName,
		},
		{
			spec: NewJWTPayloadAllKVRegexp(),
			name: MatchJWTPayloadAllKVRegexpName,
		},
		{
			spec: NewJWTPayloadAnyKVRegexp(),
			name: MatchJWTPayloadAnyKVRegexpName,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.NotNil(t, tc.spec)
			require.Equal(t, tc.name, tc.spec.Name())
		})
	}
}

func exact(expected string) exactMatcher {
	return exactMatcher{expected: expected}
}

func regex(pattern string) regexMatcher {
	return regexMatcher{regexp: regexp.MustCompile(pattern)}
}

func Test_spec_Create(t *testing.T) {
	tests := []struct {
		name    string
		spec    routing.PredicateSpec
		args    []interface{}
		want    routing.Predicate
		wantErr bool
	}{{
		name:    "invalid number of args",
		spec:    NewJWTPayloadAllKV(),
		args:    []interface{}{"foo"},
		want:    nil,
		wantErr: true,
	}, {
		name:    "invalid type of args",
		spec:    NewJWTPayloadAllKV(),
		args:    []interface{}{3, 5},
		want:    nil,
		wantErr: true,
	}, {
		name: "one valid kv pair of args",
		spec: NewJWTPayloadAllKV(),
		args: []interface{}{"uid", "sszuecs"},
		want: &predicate{
			kv: map[string][]valueMatcher{
				"uid": {exact("sszuecs")},
			},
			matchBehavior: matchBehaviorAll,
		},
		wantErr: false,
	}, {
		name: "one valid kv pair of args",
		spec: NewJWTPayloadAnyKV(),
		args: []interface{}{"uid", "sszuecs"},
		want: &predicate{
			kv: map[string][]valueMatcher{
				"uid": {exact("sszuecs")},
			},
			matchBehavior: matchBehaviorAny,
		},
		wantErr: false,
	}, {
		name: "valid kv pair of args of the same key",
		spec: NewJWTPayloadAnyKV(),
		args: []interface{}{"uid", "sszuecs", "uid", "foo"},
		want: &predicate{
			kv: map[string][]valueMatcher{
				"uid": {exact("sszuecs"), exact("foo")},
			},
			matchBehavior: matchBehaviorAny,
		},
		wantErr: false,
	}, {
		name: "many valid kv pair of args",
		spec: NewJWTPayloadAllKV(),
		args: []interface{}{"uid", "sszuecs", "claim1", "claimValue1", "claim2", "claimValue2"},
		want: &predicate{
			kv: map[string][]valueMatcher{
				"uid":    {exact("sszuecs")},
				"claim1": {exact("claimValue1")},
				"claim2": {exact("claimValue2")},
			},
			matchBehavior: matchBehaviorAll,
		},
		wantErr: false,
	}, {
		name: "many valid kv pair of args",
		spec: NewJWTPayloadAnyKV(),
		args: []interface{}{"uid", "sszuecs", "claim1", "claimValue1", "claim2", "claimValue2"},
		want: &predicate{
			kv: map[string][]valueMatcher{
				"uid":    {exact("sszuecs")},
				"claim1": {exact("claimValue1")},
				"claim2": {exact("claimValue2")},
			},
			matchBehavior: matchBehaviorAny,
		},
		wantErr: false,
	}, {
		name: "many valid kv pair of args, regexp matching",
		spec: NewJWTPayloadAllKVRegexp(),
		args: []interface{}{"uid", "sszuecs", "claim1", "claimValue1", "claim2", "claimValue2"},
		want: &predicate{
			kv: map[string][]valueMatcher{
				"uid":    {regex("sszuecs")},
				"claim1": {regex("claimValue1")},
				"claim2": {regex("claimValue2")},
			},
			matchBehavior: matchBehaviorAll,
		},
		wantErr: false,
	}, {
		name: "many valid kv pair of args, regexp matching",
		spec: NewJWTPayloadAnyKVRegexp(),
		args: []interface{}{"uid", "sszuecs", "claim1", "claimValue1", "claim2", "claimValue2"},
		want: &predicate{
			kv: map[string][]valueMatcher{
				"uid":    {regex("sszuecs")},
				"claim1": {regex("claimValue1")},
				"claim2": {regex("claimValue2")},
			},
			matchBehavior: matchBehaviorAny,
		},
		wantErr: false,
	}, {
		name:    "many kv pair of args, one missing",
		spec:    NewJWTPayloadAllKV(),
		args:    []interface{}{"uid", "sszuecs", "claim1", "claimValue1", "claim2"},
		want:    nil,
		wantErr: true,
	}, {
		name:    "many kv pair of args",
		spec:    NewJWTPayloadAnyKV(),
		args:    []interface{}{"uid", "sszuecs", "claim1", "claimValue1", "claim2"},
		want:    nil,
		wantErr: true,
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, ok := tt.spec.(*spec)
			if !ok {
				t.Errorf("unexpected spec value: %v", tt.spec)
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
		kv   map[string][]valueMatcher
		tok  string
		want bool
	}{{
		name: "no valid kv pairs matching",
		kv: map[string][]valueMatcher{
			"https://identity.zalando.com/managed-id": {exact("sszuecs")},
			"https://identity.zalando.com/token":      {exact("Bearer")},
		},
		tok:  "eyJraWQiOiJwbGF0Zm9ybS1pYW0tdmNlaHloajYiLCJhbGciOiJFUzI1NiJ9.eyJzdWIiOiJjNGRkZmU5ZC1hMGQzLTRhZmItYmYyNi0yNGI5NTg4NzMxYTAiLCJodHRwczovL2lkZW50aXR5LnphbGFuZG8uY29tL3JlYWxtIjoidXNlcnMiLCJodHRwczovL2lkZW50aXR5LnphbGFuZG8uY29tL3Rva2VuIjoiQmVhcmVyIiwiaHR0cHM6Ly9pZGVudGl0eS56YWxhbmRvLmNvbS9tYW5hZ2VkLWlkIjoic3N6dWVjcyIsImF6cCI6Inp0b2tlbiIsImh0dHBzOi8vaWRlbnRpdHkuemFsYW5kby5jb20vYnAiOiI4MTBkMWQwMC00MzEyLTQzZTUtYmQzMS1kODM3M2ZkZDI0YzciLCJhdXRoX3RpbWUiOjE1MjMyNTk0NjgsImlzcyI6Imh0dHBzOi8vaWRlbnRpdHkuemFsYW5kby5jb20iLCJleHAiOjE1MjUwMjQyODUsImlhdCI6MTUyNTAyMDY3NX0.uxHcC7DJrkP-_G81Jmiba5liVP0LJOmkpal4wsUr7CmtMlE23P1bptIMxnJLv5EMSN1NFn-BJe9hcEB2A3LarA",
		want: true,
	}, {
		name: "many valid kv pairs matching",
		kv: map[string][]valueMatcher{
			"https://identity.zalando.com/managed-id": {exact("sszuecs")},
			"https://identity.zalando.com/token":      {exact("Bearer")},
		},
		tok:  "eyJraWQiOiJwbGF0Zm9ybS1pYW0tdmNlaHloajYiLCJhbGciOiJFUzI1NiJ9.eyJzdWIiOiJjNGRkZmU5ZC1hMGQzLTRhZmItYmYyNi0yNGI5NTg4NzMxYTAiLCJodHRwczovL2lkZW50aXR5LnphbGFuZG8uY29tL3JlYWxtIjoidXNlcnMiLCJodHRwczovL2lkZW50aXR5LnphbGFuZG8uY29tL3Rva2VuIjoiQmVhcmVyIiwiaHR0cHM6Ly9pZGVudGl0eS56YWxhbmRvLmNvbS9tYW5hZ2VkLWlkIjoic3N6dWVjcyIsImF6cCI6Inp0b2tlbiIsImh0dHBzOi8vaWRlbnRpdHkuemFsYW5kby5jb20vYnAiOiI4MTBkMWQwMC00MzEyLTQzZTUtYmQzMS1kODM3M2ZkZDI0YzciLCJhdXRoX3RpbWUiOjE1MjMyNTk0NjgsImlzcyI6Imh0dHBzOi8vaWRlbnRpdHkuemFsYW5kby5jb20iLCJleHAiOjE1MjUwMjQyODUsImlhdCI6MTUyNTAyMDY3NX0.uxHcC7DJrkP-_G81Jmiba5liVP0LJOmkpal4wsUr7CmtMlE23P1bptIMxnJLv5EMSN1NFn-BJe9hcEB2A3LarA",
		want: true,
	}, {
		name: "many valid kv pairs matching (regexp)",
		kv: map[string][]valueMatcher{
			"https://identity.zalando.com/managed-id": {regex("^ssz")},
			"https://identity.zalando.com/token":      {regex("^Bear")},
		},
		tok:  "eyJraWQiOiJwbGF0Zm9ybS1pYW0tdmNlaHloajYiLCJhbGciOiJFUzI1NiJ9.eyJzdWIiOiJjNGRkZmU5ZC1hMGQzLTRhZmItYmYyNi0yNGI5NTg4NzMxYTAiLCJodHRwczovL2lkZW50aXR5LnphbGFuZG8uY29tL3JlYWxtIjoidXNlcnMiLCJodHRwczovL2lkZW50aXR5LnphbGFuZG8uY29tL3Rva2VuIjoiQmVhcmVyIiwiaHR0cHM6Ly9pZGVudGl0eS56YWxhbmRvLmNvbS9tYW5hZ2VkLWlkIjoic3N6dWVjcyIsImF6cCI6Inp0b2tlbiIsImh0dHBzOi8vaWRlbnRpdHkuemFsYW5kby5jb20vYnAiOiI4MTBkMWQwMC00MzEyLTQzZTUtYmQzMS1kODM3M2ZkZDI0YzciLCJhdXRoX3RpbWUiOjE1MjMyNTk0NjgsImlzcyI6Imh0dHBzOi8vaWRlbnRpdHkuemFsYW5kby5jb20iLCJleHAiOjE1MjUwMjQyODUsImlhdCI6MTUyNTAyMDY3NX0.uxHcC7DJrkP-_G81Jmiba5liVP0LJOmkpal4wsUr7CmtMlE23P1bptIMxnJLv5EMSN1NFn-BJe9hcEB2A3LarA",
		want: true,
	}, {
		name: "many valid kv pairs invalid token content",
		kv: map[string][]valueMatcher{
			"https://identity.zalando.com/managed-id": {exact("sszuecs")},
			"https://identity.zalando.com/token":      {exact("Bearer")},
		},
		tok:  "eyJraWQiOiJwbGF0Zm9ybS1pYW0tdmNlaHloajYiLCJhbGciOiJFUzI1NiJ9.31JzdWIiOiJjNG34ZmU5ZC1hMGQzLTRhZmItYmYyNi0yNGI5NTg4NzMxYTAiLCJodHRwczovL2lkZW50aXR5LnphbGFuZG8uY29tL3JlYWxtIjoidXNlcnMiLCJodHRwczovL2lkZW50aXR5LnphbGFuZG8uY29tL3Rva2VuIjoiQmVhcmVyIiwiaHR0cHM6Ly9pZGVudGl0eS56YWxhbmRvLmNvbS9tYW5hZ2VkLWlkIjoic3N6dWVjcyIsImF6cCI6Inp0b2tlbiIsImh0dHBzOi8vaWRlbnRpdHkuemFsYW5kby5jb20vYnAiOiI4MTBkMWQwMC00MzEyLTQzZTUtYmQzMS1kODM3M2ZkZDI0YzciLCJhdXRoX3RpbWUiOjE1MjMyNTk0NjgsImlzcyI6Imh0dHBzOi8vaWRlbnRpdHkuemFsYW5kby5jb20iLCJleHAiOjE1MjUwMjQyODUsImlhdCI6MTUyNTAyMDY3NX0.uxHcC7DJrkP-_G81Jmiba5liVP0LJOmkpal4wsUr7CmtMlE23P1bptIMxnJLv5EMSN1NFn-BJe9hcEB2A3LarA",
		want: false,
	}, {
		name: "many valid kv pairs invalid token fields",
		kv: map[string][]valueMatcher{
			"https://identity.zalando.com/managed-id": {exact("sszuecs")},
			"https://identity.zalando.com/token":      {exact("Bearer")},
		},
		tok:  "eyJraWQiOiJwbGF0Zm9ybS1pYW0tdmNlaHloajYiLCJhbGciOiJFUzI1NiJ9.eyJzdWIiOiJjNGRkZmU5ZC1hMGQzLTRhZmItYmYyNi0yNGI5NTg4NzMxYTAiLCJodHRwczovL2lkZW50aXR5LnphbGFuZG8uY29tL3JlYWxtIjoidXNlcnMiLCJodHRwczovL2lkZW.50aXR5LnphbGFuZG8uY29tL3Rva2VuIjoiQmVhcmVyIiwiaHR0cHM6Ly9pZGVudGl0eS56YWxhbmRvLmNvbS9tYW5hZ2VkLWlkIjoic3N6dWVjcyIsImF6cCI6Inp0b2tlbiIsImh0dHBzOi8vaWRlbnRpdHkuemFsYW5kby5jb20vYnAiOiI4MTBkMWQwMC00MzEyLTQzZTUtYmQzMS1kODM3M2ZkZDI0YzciLCJhdXRoX3RpbWUiOjE1MjMyNTk0NjgsImlzcyI6Imh0dHBzOi8vaWRlbnRpdHkuemFsYW5kby5jb20iLCJleHAiOjE1MjUwMjQyODUsImlhdCI6MTUyNTAyMDY3NX0.uxHcC7DJrkP-_G81Jmiba5liVP0LJOmkpal4wsUr7CmtMlE23P1bptIMxnJLv5EMSN1NFn-BJe9hcEB2A3LarA",
		want: false,
	}, {
		name: "many valid kv pairs invalid base64 in token field",
		kv: map[string][]valueMatcher{
			"https://identity.zalando.com/managed-id": {exact("sszuecs")},
			"https://identity.zalando.com/token":      {exact("Bearer")},
		},
		tok:  "eyJraWQiOiJwbGF0Zm9ybS1pYW0tdmNlaHloajYiLCJhbGciOiJFUzI1NiJ9.eyJzdWIiOiJjNGRkZmU5ZC1hMGQzLTRhZmItYmYyNi0yNGI5NTg4NzMxYTAiLCJodHRwczovL2lkZW50aXR5LnphbGFuZG8uY29tL3JlYWxtIjoidXNlcnMiLCJodHRwczovL2lkZ_50aXR5LnphbGFuZG8uY29tL3Rva2VuIjoiQmVhcmVyIiwiaHR0cHM6Ly9pZGVudGl0eS56YWxhbmRvLmNvbS9tYW5hZ2VkLWlkIjoic3N6dWVjcyIsImF6cCI6Inp0b2tlbiIsImh0dHBzOi8vaWRlbnRpdHkuemFsYW5kby5jb20vYnAiOiI4MTBkMWQwMC00MzEyLTQzZTUtYmQzMS1kODM3M2ZkZDI0YzciLCJhdXRoX3RpbWUiOjE1MjMyNTk0NjgsImlzcyI6Imh0dHBzOi8vaWRlbnRpdHkuemFsYW5kby5jb20iLCJleHAiOjE1MjUwMjQyODUsImlhdCI6MTUyNTAyMDY3NX0.uxHcC7DJrkP-_G81Jmiba5liVP0LJOmkpal4wsUr7CmtMlE23P1bptIMxnJLv5EMSN1NFn-BJe9hcEB2A3LarA",
		want: false,
	}, {
		name: "many valid kv pairs invalid managed-id in token",
		kv: map[string][]valueMatcher{
			"https://identity.zalando.com/managed-id": {exact("sszuecs")},
			"https://identity.zalando.com/token":      {exact("Bearer")},
		},
		tok:  "eyJraWQiOiJwbGF0Zm9ybS1pYW0tdmNlaHloajYiLCJhbGciOiJFUzI1NiJ9.eyJzdWIiOiJjNGRkZmU5ZC1hMGQzLTRhZmItYmYyNi0yNGI5NTg4NzMxYTAiLCJodHRwczovL2lkZW50aXR5LnphbGFuZG8uY29tL3JlYWxtIjoidXNlcnMiLCJodHRwczovL2lkZW50aXR5LnphbGFuZG8uY29tL3Rva2VuIjoiQmVhcmVyIiwiaHR0cHM6Ly9pZGVudGl0eS56YWxhbmRvLmNvbS9tYW5hZ2VkLWlkIjoic29tZW9uZSIsImF6cCI6Inp0b2tlbiIsImh0dHBzOi8vaWRlbnRpdHkuemFsYW5kby5jb20vYnAiOiI4MTBkMWQwMC00MzEyLTQzZTUtYmQzMS1kODM3M2ZkZDI0YzciLCJhdXRoX3RpbWUiOjE1MjMyNTk0NjgsImlzcyI6Imh0dHBzOi8vaWRlbnRpdHkuemFsYW5kby5jb20iLCJleHAiOjE1MjUwMjQyODUsImlhdCI6MTUyNTAyMDY3NX0K.uxHcC7DJrkP-_G81Jmiba5liVP0LJOmkpal4wsUr7CmtMlE23P1bptIMxnJLv5EMSN1NFn-BJe9hcEB2A3LarA",
		want: false,
	}, {
		name: "many valid kv pairs invalid managed-id in token (prefix)",
		kv: map[string][]valueMatcher{
			"https://identity.zalando.com/managed-id": {regex("^ssz")},
			"https://identity.zalando.com/token":      {regex("^Bearer$")},
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
			p := &predicate{
				kv:            tt.kv,
				matchBehavior: matchBehaviorAll,
			}
			if got := p.Match(r); got != tt.want {
				t.Errorf("predicateAll.Match() = %v, want %v", got, tt.want)
			}
		})
	}

	t.Run("wrong header prefix", func(t *testing.T) {
		r := &http.Request{
			Header: http.Header{
				authHeaderName: []string{"Foo " + "eyJraWQiOiJwbGF0Zm9ybS1pYW0tdmNlaHloajYiLCJhbGciOiJFUzI1NiJ9.eyJzdWIiOiJjNGRkZmU5ZC1hMGQzLTRhZmItYmYyNi0yNGI5NTg4NzMxYTAiLCJodHRwczovL2lkZW50aXR5LnphbGFuZG8uY29tL3JlYWxtIjoidXNlcnMiLCJodHRwczovL2lkZW50aXR5LnphbGFuZG8uY29tL3Rva2VuIjoiQmVhcmVyIiwiaHR0cHM6Ly9pZGVudGl0eS56YWxhbmRvLmNvbS9tYW5hZ2VkLWlkIjoic29tZW9uZSIsImF6cCI6Inp0b2tlbiIsImh0dHBzOi8vaWRlbnRpdHkuemFsYW5kby5jb20vYnAiOiI4MTBkMWQwMC00MzEyLTQzZTUtYmQzMS1kODM3M2ZkZDI0YzciLCJhdXRoX3RpbWUiOjE1MjMyNTk0NjgsImlzcyI6Imh0dHBzOi8vaWRlbnRpdHkuemFsYW5kby5jb20iLCJleHAiOjE1MjUwMjQyODUsImlhdCI6MTUyNTAyMDY3NX0K.uxHcC7DJrkP-_G81Jmiba5liVP0LJOmkpal4wsUr7CmtMlE23P1bptIMxnJLv5EMSN1NFn-BJe9hcEB2A3LarA"},
			},
		}
		p := &predicate{
			kv: map[string][]valueMatcher{
				"https://identity.zalando.com/managed-id": {exact("sszuecs")},
				"https://identity.zalando.com/token":      {exact("Bearer")},
			},
			matchBehavior: matchBehaviorAll,
		}
		if got := p.Match(r); got != false {
			t.Error("predicateAll.Match() should not match if there is not a matching header")
		}
	})
}

func Test_predicateAny_Match(t *testing.T) {
	tests := []struct {
		name string
		kv   map[string][]valueMatcher
		tok  string
		want bool
	}{{
		name: "no valid kv pairs matching",
		kv: map[string][]valueMatcher{
			"https://identity.zalando.com/managed-id": {exact("sszuecs")},
			"https://identity.zalando.com/token":      {exact("Bearer")},
		},
		tok:  "eyJraWQiOiJwbGF0Zm9ybS1pYW0tdmNlaHloajYiLCJhbGciOiJFUzI1NiJ9.eyJzdWIiOiJjNGRkZmU5ZC1hMGQzLTRhZmItYmYyNi0yNGI5NTg4NzMxYTAiLCJodHRwczovL2lkZW50aXR5LnphbGFuZG8uY29tL3JlYWxtIjoidXNlcnMiLCJodHRwczovL2lkZW50aXR5LnphbGFuZG8uY29tL3Rva2VuIjoiQmVhcmVyIiwiaHR0cHM6Ly9pZGVudGl0eS56YWxhbmRvLmNvbS9tYW5hZ2VkLWlkIjoic3N6dWVjcyIsImF6cCI6Inp0b2tlbiIsImh0dHBzOi8vaWRlbnRpdHkuemFsYW5kby5jb20vYnAiOiI4MTBkMWQwMC00MzEyLTQzZTUtYmQzMS1kODM3M2ZkZDI0YzciLCJhdXRoX3RpbWUiOjE1MjMyNTk0NjgsImlzcyI6Imh0dHBzOi8vaWRlbnRpdHkuemFsYW5kby5jb20iLCJleHAiOjE1MjUwMjQyODUsImlhdCI6MTUyNTAyMDY3NX0.uxHcC7DJrkP-_G81Jmiba5liVP0LJOmkpal4wsUr7CmtMlE23P1bptIMxnJLv5EMSN1NFn-BJe9hcEB2A3LarA",
		want: true,
	}, {
		name: "many valid kv pairs matching",
		kv: map[string][]valueMatcher{
			"https://identity.zalando.com/managed-id": {exact("sszuecs")},
			"https://identity.zalando.com/token":      {exact("Bearer")},
		},
		tok:  "eyJraWQiOiJwbGF0Zm9ybS1pYW0tdmNlaHloajYiLCJhbGciOiJFUzI1NiJ9.eyJzdWIiOiJjNGRkZmU5ZC1hMGQzLTRhZmItYmYyNi0yNGI5NTg4NzMxYTAiLCJodHRwczovL2lkZW50aXR5LnphbGFuZG8uY29tL3JlYWxtIjoidXNlcnMiLCJodHRwczovL2lkZW50aXR5LnphbGFuZG8uY29tL3Rva2VuIjoiQmVhcmVyIiwiaHR0cHM6Ly9pZGVudGl0eS56YWxhbmRvLmNvbS9tYW5hZ2VkLWlkIjoic3N6dWVjcyIsImF6cCI6Inp0b2tlbiIsImh0dHBzOi8vaWRlbnRpdHkuemFsYW5kby5jb20vYnAiOiI4MTBkMWQwMC00MzEyLTQzZTUtYmQzMS1kODM3M2ZkZDI0YzciLCJhdXRoX3RpbWUiOjE1MjMyNTk0NjgsImlzcyI6Imh0dHBzOi8vaWRlbnRpdHkuemFsYW5kby5jb20iLCJleHAiOjE1MjUwMjQyODUsImlhdCI6MTUyNTAyMDY3NX0.uxHcC7DJrkP-_G81Jmiba5liVP0LJOmkpal4wsUr7CmtMlE23P1bptIMxnJLv5EMSN1NFn-BJe9hcEB2A3LarA",
		want: true,
	}, {
		name: "many valid kv pairs matching (prefix)",
		kv: map[string][]valueMatcher{
			"https://identity.zalando.com/managed-id": {regex("ssz")},
			"https://identity.zalando.com/token":      {regex("Bear")},
		},
		tok:  "eyJraWQiOiJwbGF0Zm9ybS1pYW0tdmNlaHloajYiLCJhbGciOiJFUzI1NiJ9.eyJzdWIiOiJjNGRkZmU5ZC1hMGQzLTRhZmItYmYyNi0yNGI5NTg4NzMxYTAiLCJodHRwczovL2lkZW50aXR5LnphbGFuZG8uY29tL3JlYWxtIjoidXNlcnMiLCJodHRwczovL2lkZW50aXR5LnphbGFuZG8uY29tL3Rva2VuIjoiQmVhcmVyIiwiaHR0cHM6Ly9pZGVudGl0eS56YWxhbmRvLmNvbS9tYW5hZ2VkLWlkIjoic3N6dWVjcyIsImF6cCI6Inp0b2tlbiIsImh0dHBzOi8vaWRlbnRpdHkuemFsYW5kby5jb20vYnAiOiI4MTBkMWQwMC00MzEyLTQzZTUtYmQzMS1kODM3M2ZkZDI0YzciLCJhdXRoX3RpbWUiOjE1MjMyNTk0NjgsImlzcyI6Imh0dHBzOi8vaWRlbnRpdHkuemFsYW5kby5jb20iLCJleHAiOjE1MjUwMjQyODUsImlhdCI6MTUyNTAyMDY3NX0.uxHcC7DJrkP-_G81Jmiba5liVP0LJOmkpal4wsUr7CmtMlE23P1bptIMxnJLv5EMSN1NFn-BJe9hcEB2A3LarA",
		want: true,
	}, {
		name: "many valid kv pairs matching",
		kv: map[string][]valueMatcher{
			"https://identity.zalando.com/managed-id": {exact("sszuecs")},
			"https://identity.zalando.com/token":      {exact("Bearer")},
		},
		tok:  "eyJraWQiOiJwbGF0Zm9ybS1pYW0tdmNlaHloajYiLCJhbGciOiJFUzI1NiJ9.eyJzdWIiOiJjNGRkZmU5ZC1hMGQzLTRhZmItYmYyNi0yNGI5NTg4NzMxYTAiLCJodHRwczovL2lkZW50aXR5LnphbGFuZG8uY29tL3JlYWxtIjoidXNlcnMiLCJodHRwczovL2lkZW50aXR5LnphbGFuZG8uY29tL3Rva2VuIjoiQmVhcmVyIiwiaHR0cHM6Ly9pZGVudGl0eS56YWxhbmRvLmNvbS9tYW5hZ2VkLWlkIjoic3N6dWVjcyIsImF6cCI6Inp0b2tlbiIsImh0dHBzOi8vaWRlbnRpdHkuemFsYW5kby5jb20vYnAiOiI4MTBkMWQwMC00MzEyLTQzZTUtYmQzMS1kODM3M2ZkZDI0YzciLCJhdXRoX3RpbWUiOjE1MjMyNTk0NjgsImlzcyI6Imh0dHBzOi8vaWRlbnRpdHkuemFsYW5kby5jb20iLCJleHAiOjE1MjUwMjQyODUsImlhdCI6MTUyNTAyMDY3NX0.uxHcC7DJrkP-_G81Jmiba5liVP0LJOmkpal4wsUr7CmtMlE23P1bptIMxnJLv5EMSN1NFn-BJe9hcEB2A3LarA",
		want: true,
	}, {
		name: "one matching managed-id token in kv pair",
		kv: map[string][]valueMatcher{
			"https://identity.zalando.com/managed-id": {exact("foo"), exact("sszuecs"), exact("bar")},
		},
		tok:  "eyJraWQiOiJwbGF0Zm9ybS1pYW0tdmNlaHloajYiLCJhbGciOiJFUzI1NiJ9.eyJzdWIiOiJjNGRkZmU5ZC1hMGQzLTRhZmItYmYyNi0yNGI5NTg4NzMxYTAiLCJodHRwczovL2lkZW50aXR5LnphbGFuZG8uY29tL3JlYWxtIjoidXNlcnMiLCJodHRwczovL2lkZW50aXR5LnphbGFuZG8uY29tL3Rva2VuIjoiQmVhcmVyIiwiaHR0cHM6Ly9pZGVudGl0eS56YWxhbmRvLmNvbS9tYW5hZ2VkLWlkIjoic3N6dWVjcyIsImF6cCI6Inp0b2tlbiIsImh0dHBzOi8vaWRlbnRpdHkuemFsYW5kby5jb20vYnAiOiI4MTBkMWQwMC00MzEyLTQzZTUtYmQzMS1kODM3M2ZkZDI0YzciLCJhdXRoX3RpbWUiOjE1MjMyNTk0NjgsImlzcyI6Imh0dHBzOi8vaWRlbnRpdHkuemFsYW5kby5jb20iLCJleHAiOjE1MjUwMjQyODUsImlhdCI6MTUyNTAyMDY3NX0.uxHcC7DJrkP-_G81Jmiba5liVP0LJOmkpal4wsUr7CmtMlE23P1bptIMxnJLv5EMSN1NFn-BJe9hcEB2A3LarA",
		want: true,
	}, {
		name: "one matching managed-id token in kv pair (regexp)",
		kv: map[string][]valueMatcher{
			"https://identity.zalando.com/managed-id": {regex("foo"), regex("^ssz"), regex("bar")},
		},
		tok:  "eyJraWQiOiJwbGF0Zm9ybS1pYW0tdmNlaHloajYiLCJhbGciOiJFUzI1NiJ9.eyJzdWIiOiJjNGRkZmU5ZC1hMGQzLTRhZmItYmYyNi0yNGI5NTg4NzMxYTAiLCJodHRwczovL2lkZW50aXR5LnphbGFuZG8uY29tL3JlYWxtIjoidXNlcnMiLCJodHRwczovL2lkZW50aXR5LnphbGFuZG8uY29tL3Rva2VuIjoiQmVhcmVyIiwiaHR0cHM6Ly9pZGVudGl0eS56YWxhbmRvLmNvbS9tYW5hZ2VkLWlkIjoic3N6dWVjcyIsImF6cCI6Inp0b2tlbiIsImh0dHBzOi8vaWRlbnRpdHkuemFsYW5kby5jb20vYnAiOiI4MTBkMWQwMC00MzEyLTQzZTUtYmQzMS1kODM3M2ZkZDI0YzciLCJhdXRoX3RpbWUiOjE1MjMyNTk0NjgsImlzcyI6Imh0dHBzOi8vaWRlbnRpdHkuemFsYW5kby5jb20iLCJleHAiOjE1MjUwMjQyODUsImlhdCI6MTUyNTAyMDY3NX0.uxHcC7DJrkP-_G81Jmiba5liVP0LJOmkpal4wsUr7CmtMlE23P1bptIMxnJLv5EMSN1NFn-BJe9hcEB2A3LarA",
		want: true,
	}, {
		name: "one valid managed-id kv pair invalid token content",
		kv: map[string][]valueMatcher{
			"https://identity.zalando.com/managed-id": {exact("foo"), exact("sszuecs"), exact("bar")},
		},
		tok:  "eyJraWQiOiJwbGF0Zm9ybS1pYW0tdmNlaHloajYiLCJhbGciOiJFUzI1NiJ9.31JzdWIiOiJjNG34ZmU5ZC1hMGQzLTRhZmItYmYyNi0yNGI5NTg4NzMxYTAiLCJodHRwczovL2lkZW50aXR5LnphbGFuZG8uY29tL3JlYWxtIjoidXNlcnMiLCJodHRwczovL2lkZW50aXR5LnphbGFuZG8uY29tL3Rva2VuIjoiQmVhcmVyIiwiaHR0cHM6Ly9pZGVudGl0eS56YWxhbmRvLmNvbS9tYW5hZ2VkLWlkIjoic3N6dWVjcyIsImF6cCI6Inp0b2tlbiIsImh0dHBzOi8vaWRlbnRpdHkuemFsYW5kby5jb20vYnAiOiI4MTBkMWQwMC00MzEyLTQzZTUtYmQzMS1kODM3M2ZkZDI0YzciLCJhdXRoX3RpbWUiOjE1MjMyNTk0NjgsImlzcyI6Imh0dHBzOi8vaWRlbnRpdHkuemFsYW5kby5jb20iLCJleHAiOjE1MjUwMjQyODUsImlhdCI6MTUyNTAyMDY3NX0.uxHcC7DJrkP-_G81Jmiba5liVP0LJOmkpal4wsUr7CmtMlE23P1bptIMxnJLv5EMSN1NFn-BJe9hcEB2A3LarA",
		want: false,
	}, {
		name: "valid kv pair invalid token fields",
		kv: map[string][]valueMatcher{
			"https://identity.zalando.com/managed-id": {exact("sszuecs")},
		},
		tok:  "eyJraWQiOiJwbGF0Zm9ybS1pYW0tdmNlaHloajYiLCJhbGciOiJFUzI1NiJ9.eyJzdWIiOiJjNGRkZmU5ZC1hMGQzLTRhZmItYmYyNi0yNGI5NTg4NzMxYTAiLCJodHRwczovL2lkZW50aXR5LnphbGFuZG8uY29tL3JlYWxtIjoidXNlcnMiLCJodHRwczovL2lkZW.50aXR5LnphbGFuZG8uY29tL3Rva2VuIjoiQmVhcmVyIiwiaHR0cHM6Ly9pZGVudGl0eS56YWxhbmRvLmNvbS9tYW5hZ2VkLWlkIjoic3N6dWVjcyIsImF6cCI6Inp0b2tlbiIsImh0dHBzOi8vaWRlbnRpdHkuemFsYW5kby5jb20vYnAiOiI4MTBkMWQwMC00MzEyLTQzZTUtYmQzMS1kODM3M2ZkZDI0YzciLCJhdXRoX3RpbWUiOjE1MjMyNTk0NjgsImlzcyI6Imh0dHBzOi8vaWRlbnRpdHkuemFsYW5kby5jb20iLCJleHAiOjE1MjUwMjQyODUsImlhdCI6MTUyNTAyMDY3NX0.uxHcC7DJrkP-_G81Jmiba5liVP0LJOmkpal4wsUr7CmtMlE23P1bptIMxnJLv5EMSN1NFn-BJe9hcEB2A3LarA",
		want: false,
	}, {
		name: "valid kv pair invalid token fields (regexp)",
		kv: map[string][]valueMatcher{
			"https://identity.zalando.com/managed-id": {regex("^ssz")},
		},
		tok:  "eyJraWQiOiJwbGF0Zm9ybS1pYW0tdmNlaHloajYiLCJhbGciOiJFUzI1NiJ9.eyJzdWIiOiJjNGRkZmU5ZC1hMGQzLTRhZmItYmYyNi0yNGI5NTg4NzMxYTAiLCJodHRwczovL2lkZW50aXR5LnphbGFuZG8uY29tL3JlYWxtIjoidXNlcnMiLCJodHRwczovL2lkZW.50aXR5LnphbGFuZG8uY29tL3Rva2VuIjoiQmVhcmVyIiwiaHR0cHM6Ly9pZGVudGl0eS56YWxhbmRvLmNvbS9tYW5hZ2VkLWlkIjoic3N6dWVjcyIsImF6cCI6Inp0b2tlbiIsImh0dHBzOi8vaWRlbnRpdHkuemFsYW5kby5jb20vYnAiOiI4MTBkMWQwMC00MzEyLTQzZTUtYmQzMS1kODM3M2ZkZDI0YzciLCJhdXRoX3RpbWUiOjE1MjMyNTk0NjgsImlzcyI6Imh0dHBzOi8vaWRlbnRpdHkuemFsYW5kby5jb20iLCJleHAiOjE1MjUwMjQyODUsImlhdCI6MTUyNTAyMDY3NX0.uxHcC7DJrkP-_G81Jmiba5liVP0LJOmkpal4wsUr7CmtMlE23P1bptIMxnJLv5EMSN1NFn-BJe9hcEB2A3LarA",
		want: false,
	}, {
		name: "valid kv pair invalid base64 in token field",
		kv: map[string][]valueMatcher{
			"https://identity.zalando.com/managed-id": {exact("sszuecs")},
		},
		tok:  "eyJraWQiOiJwbGF0Zm9ybS1pYW0tdmNlaHloajYiLCJhbGciOiJFUzI1NiJ9.eyJzdWIiOiJjNGRkZmU5ZC1hMGQzLTRhZmItYmYyNi0yNGI5NTg4NzMxYTAiLCJodHRwczovL2lkZW50aXR5LnphbGFuZG8uY29tL3JlYWxtIjoidXNlcnMiLCJodHRwczovL2lkZ__50aXR5LnphbGFuZG8uY29tL3Rva2VuIjoiQmVhcmVyIiwiaHR0cHM6Ly9pZGVudGl0eS56YWxhbmRvLmNvbS9tYW5hZ2VkLWlkIjoic3N6dWVjcyIsImF6cCI6Inp0b2tlbiIsImh0dHBzOi8vaWRlbnRpdHkuemFsYW5kby5jb20vYnAiOiI4MTBkMWQwMC00MzEyLTQzZTUtYmQzMS1kODM3M2ZkZDI0YzciLCJhdXRoX3RpbWUiOjE1MjMyNTk0NjgsImlzcyI6Imh0dHBzOi8vaWRlbnRpdHkuemFsYW5kby5jb20iLCJleHAiOjE1MjUwMjQyODUsImlhdCI6MTUyNTAyMDY3NX0.uxHcC7DJrkP-_G81Jmiba5liVP0LJOmkpal4wsUr7CmtMlE23P1bptIMxnJLv5EMSN1NFn-BJe9hcEB2A3LarA",
		want: false,
	}, {
		name: "many valid kv pairs invalid managed-id in token",
		kv: map[string][]valueMatcher{
			"https://identity.zalando.com/managed-id": {exact("sszuecs")},
			"https://identity.zalando.com/token":      {exact("Bearer")},
		},
		tok:  "eyJraWQiOiJwbGF0Zm9ybS1pYW0tdmNlaHloajYiLCJhbGciOiJFUzI1NiJ9.eyJzdWIiOiJjNGRkZmU5ZC1hMGQzLTRhZmItYmYyNi0yNGI5NTg4NzMxYTAiLCJodHRwczovL2lkZW50aXR5LnphbGFuZG8uY29tL3JlYWxtIjoidXNlcnMiLCJodHRwczovL2lkZW50aXR5LnphbGFuZG8uY29tL3Rva2VuIjoiQmVhcmVyIiwiaHR0cHM6Ly9pZGVudGl0eS56YWxhbmRvLmNvbS9tYW5hZ2VkLWlkIjoic29tZW9uZSIsImF6cCI6Inp0b2tlbiIsImh0dHBzOi8vaWRlbnRpdHkuemFsYW5kby5jb20vYnAiOiI4MTBkMWQwMC00MzEyLTQzZTUtYmQzMS1kODM3M2ZkZDI0YzciLCJhdXRoX3RpbWUiOjE1MjMyNTk0NjgsImlzcyI6Imh0dHBzOi8vaWRlbnRpdHkuemFsYW5kby5jb20iLCJleHAiOjE1MjUwMjQyODUsImlhdCI6MTUyNTAyMDY3NX0K.uxHcC7DJrkP-_G81Jmiba5liVP0LJOmkpal4wsUr7CmtMlE23P1bptIMxnJLv5EMSN1NFn-BJe9hcEB2A3LarA",
		want: true,
	}, {
		name: "many valid kv pairs invalid managed-ids in token",
		kv: map[string][]valueMatcher{
			"https://identity.zalando.com/managed-id": {exact("foo"), exact("sszuecs")},
			"https://identity.zalando.com/token":      {exact("Bearer")},
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
			p := &predicate{
				kv:            tt.kv,
				matchBehavior: matchBehaviorAny,
			}
			if got := p.Match(r); got != tt.want {
				t.Errorf("predicateAny.Match() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_allMatch(t *testing.T) {
	for _, tt := range []struct {
		name string
		kv   map[string][]valueMatcher
		h    map[string]interface{}
		want bool
	}{
		{
			name: "no kv nor h",
			want: true,
		}, {
			name: "no kv, but h",
			h: map[string]interface{}{
				"foo": "bar",
			},
			want: true,
		}, {
			name: "kv, but no h",
			kv: map[string][]valueMatcher{
				"foo": {exact("bar")},
			},
			want: false,
		}, {
			name: "multiple kv, with all overlapping h",
			kv: map[string][]valueMatcher{
				"foo": {exact("bar")},
				"x":   {exact("y")},
			},
			h: map[string]interface{}{
				"foo": "bar",
				"x":   "y",
			},
			want: true,
		}, {
			name: "multiple kv, with all overlapping h, regexp matching",
			kv: map[string][]valueMatcher{
				"foo": {regex("^b")},
				"x":   {regex("y")},
			},
			h: map[string]interface{}{
				"foo": "bar",
				"x":   "y",
			},
			want: true,
		}, {
			name: "multiple kv, with one non overlapping h",
			kv: map[string][]valueMatcher{
				"foo": {exact("bar")},
				"x":   {exact("y")},
			},
			h: map[string]interface{}{
				"foo": "bar",
				"x":   "a",
			},
			want: false,
		}} {
		t.Run(tt.name, func(t *testing.T) {
			if got := allMatch(tt.kv, tt.h); got != tt.want {
				t.Errorf("Failed to allMatch: Want %v, got %v", tt.want, got)
			}
		})
	}

}

func Test_anyMatch(t *testing.T) {
	for _, tt := range []struct {
		name string
		kv   map[string][]valueMatcher
		h    map[string]interface{}
		want bool
	}{
		{
			name: "no kv nor h",
			want: true,
		}, {
			name: "no kv, but h",
			h: map[string]interface{}{
				"foo": "bar",
			},
			want: true,
		}, {
			name: "kv, but no h",
			kv: map[string][]valueMatcher{
				"foo": {exact("bar")},
			},
			want: false,
		}, {
			name: "multiple kv, with all overlapping h",
			kv: map[string][]valueMatcher{
				"foo": {exact("bar")},
				"x":   {exact("y")},
			},
			h: map[string]interface{}{
				"foo": "bar",
				"x":   "y",
			},
			want: true,
		}, {
			name: "multiple kv, with one non overlapping h",
			kv: map[string][]valueMatcher{
				"foo": {exact("bar")},
				"x":   {exact("y")},
			},
			h: map[string]interface{}{
				"foo": "bar",
				"x":   "a",
			},
			want: true,
		}, {
			name: "multiple kv, with all overlapping h, regexp matching",
			kv: map[string][]valueMatcher{
				"foo": {regex("^b")},
				"x":   {regex("^y$")},
			},
			h: map[string]interface{}{
				"foo": "bar",
				"x":   "y",
			},
			want: true,
		}, {
			name: "multiple kv, with one non overlapping h, regexp matching",
			kv: map[string][]valueMatcher{
				"foo": {regex("^b")},
				"x":   {regex("^y$")},
			},
			h: map[string]interface{}{
				"foo": "bar",
				"x":   "a",
			},
			want: true,
		}} {
		t.Run(tt.name, func(t *testing.T) {
			if got := anyMatch(tt.kv, tt.h); got != tt.want {
				t.Errorf("Failed to anyMatch: Want %v, got %v", tt.want, got)
			}
		})
	}

}
