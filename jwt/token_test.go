package jwt

import (
	"encoding/base64"
	"encoding/json"
	"reflect"
	"testing"
)

func TestParse(t *testing.T) {
	for _, tt := range []struct {
		value  string
		ok     bool
		claims map[string]interface{}
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
			value:  "x." + marshalBase64JSON(t, map[string]interface{}{"hello": "world"}) + ".z",
			ok:     true,
			claims: map[string]interface{}{"hello": "world"},
		}, {
			value:  "." + marshalBase64JSON(t, map[string]interface{}{"no header": "no signature"}) + ".",
			ok:     true,
			claims: map[string]interface{}{"no header": "no signature"},
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

func marshalBase64JSON(t *testing.T, v interface{}) string {
	d, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("failed to marshal json: %v, %v", v, err)
	}
	return base64.RawURLEncoding.EncodeToString(d)
}
