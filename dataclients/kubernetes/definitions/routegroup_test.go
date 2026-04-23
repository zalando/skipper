package definitions

import "testing"

func TestSkipperBackendNil(t *testing.T) {
	skipperBackend := `
{
  "name": "my-backend",
  "type": "network",
  "address": "http://example"
}
`

	var sb *SkipperBackend
	if err := sb.UnmarshalJSON([]byte(skipperBackend)); err != nil {
		t.Fatalf("Failed to get nil: %q", skipperBackend)
	}
}
