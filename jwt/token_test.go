package jwt

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	for _, tt := range []struct {
		value  string
		ok     bool
		claims map[string]any
	}{
		{
			value: "",
			ok:    false,
		}, {
			value: "x",
			ok:    false,
		}, {
			value: "x.y",
			ok:    false,
		}, {
			value: "x.y.z",
			ok:    false,
		}, {
			value: "..",
			ok:    false,
		}, {
			value: "x..z",
			ok:    false,
		}, {
			value:  "x." + marshalBase64JSON(t, map[string]any{"hello": "world"}) + ".z",
			ok:     true,
			claims: map[string]any{"hello": "world"},
		}, {
			value:  "." + marshalBase64JSON(t, map[string]any{"no header": "no signature"}) + ".",
			ok:     true,
			claims: map[string]any{"no header": "no signature"},
		},
	} {
		token, err := Parse(tt.value)
		if tt.ok {
			if err != nil {
				t.Errorf("unexpected error for %s: %v", tt.value, err)
				continue
			}
		} else {
			continue
		}

		if !reflect.DeepEqual(tt.claims, token.Claims) {
			t.Errorf("claims mismatch, expected: %v, got %v", tt.claims, token.Claims)
		}
	}
}

func marshalBase64JSON(tb testing.TB, v any) string {
	d, err := json.Marshal(v)
	if err != nil {
		tb.Fatalf("failed to marshal json: %v, %v", v, err)
	}
	return base64.RawURLEncoding.EncodeToString(d)
}

var parseSink *Token

func BenchmarkParse(b *testing.B) {
	claims := map[string]any{
		"azp":                    strings.Repeat("z", 100),
		"exp":                    1234567890,
		"aaaaaaaaaaaaaaaaaaaaaa": strings.Repeat("a", 40),
		"bbbbbbbbbbbbbbbbbbbbbb": strings.Repeat("b", 40),
		"cccccccccccccccccccccc": strings.Repeat("c", 40),
		"iat":                    1234567890,
		"iss":                    "https://skipper.identity.example.org",
		"sub":                    "foo_bar-baz-qux",
	}

	value := strings.Repeat("x", 64) + "." + marshalBase64JSON(b, claims) + "." + strings.Repeat("x", 128)

	_, err := Parse(value)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		parseSink, _ = Parse(value)
	}
}

func BenchmarkParse_malicious(b *testing.B) {
	cases := []struct {
		name  string
		value string
	}{
		{
			name:  "all periods",
			value: strings.Repeat(".", http.DefaultMaxHeaderBytes),
		}, {
			name:  "two trailing periods",
			value: strings.Repeat("a", http.DefaultMaxHeaderBytes-2) + "..",
		},
	}
	for _, bc := range cases {
		b.Run(bc.name, func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				parseSink, _ = Parse(bc.value)
			}
		})
	}
}
