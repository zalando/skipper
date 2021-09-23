package rfc

import "testing"

func TestPatchHost(t *testing.T) {
	for _, tt := range []struct {
		name string
		args string
		want string
	}{
		{
			name: "test no trailing dot",
			args: "www.example.org",
			want: "www.example.org",
		},
		{
			name: "test trailing dot",
			args: "www.example.org.",
			want: "www.example.org",
		}} {
		t.Run(tt.name, func(t *testing.T) {
			if s := PatchHost(tt.args); s != tt.want {
				t.Fatalf("Failed to get the right output: %s, want: %s", s, tt.want)
			}

		})
	}

}
