package config

import (
	"testing"
)

func Test_routeChangerConfig(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "test string",
			input: "/Source[(](.*)[)]/ClientIP($1)/",
			want:  "/Source[(](.*)[)]/ClientIP($1)/",
		},
		{
			name:  "test string with # separator",
			input: "#Source[(](.*)[)]#ClientIP($1)#",
			want:  "#Source[(](.*)[)]#ClientIP($1)#",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rcc := &routeChangerConfig{}
			if err := rcc.Set(tt.input); err != nil {
				t.Errorf("Failed to parse route changer config: %v", err)
			}
			if got := rcc.String(); got != tt.want {
				t.Errorf("routeChangerConfig.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_routeChangerConfigSeveralItems(t *testing.T) {
	tests := []struct {
		name  string
		input []string
		want  string
	}{
		{
			name: "two test strings",
			input: []string{
				"/Source[(](.*)[)]/ClientIP($1)/",
				"/Destination[(](.*)[)]/ClientIP($1)/",
			},
			want: "/Source[(](.*)[)]/ClientIP($1)/\n/Destination[(](.*)[)]/ClientIP($1)/",
		},
		{
			name: "two test strings with custom separators",
			input: []string{
				"#Source[(](.*)[)]#ClientIP($1)#",
				"~Destination[(](.*)[)]~ClientIP($1)~",
			},
			want: "#Source[(](.*)[)]#ClientIP($1)#\n~Destination[(](.*)[)]~ClientIP($1)~",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rcc := &routeChangerConfig{}
			for _, str := range tt.input {
				if err := rcc.Set(str); err != nil {
					t.Errorf("Failed to parse route changer config: %v", err)
				}
			}
			if got := rcc.String(); got != tt.want {
				t.Errorf("routeChangerConfig.String() = %v, want %v", got, tt.want)
			}
		})
	}
}
