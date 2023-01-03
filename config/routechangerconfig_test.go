package config

import (
	"fmt"
	"testing"

	"gopkg.in/yaml.v2"
)

func Test_routeChangerConfig(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
		error error
	}{
		{
			name:  "empty test string",
			input: "",
			want:  "",
			error: fmt.Errorf("empty string as an argument is not allowed"),
		},
		{
			name:  "invalid test string",
			input: "/foo",
			want:  "",
			error: fmt.Errorf("unexpected size of string split: 2"),
		},
		{
			name:  "invalid regexp string",
			input: "/foo\\1/b/",
			want:  "/foo/b/",
			error: fmt.Errorf("error parsing regexp: invalid escape sequence: `\\1`"),
		},

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
			err := rcc.Set(tt.input)

			if tt.error == nil && err != nil {
				t.Errorf("Failed to parse route changer config: %v", err)
			}
			if tt.error != nil && (err == nil || tt.error.Error() != err.Error()) {
				t.Errorf("Failed to get error %v, got %v", tt.error, err)
			}
			if got := rcc.String(); err == nil && got != tt.want {
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

func TestRouteChangerYaml(t *testing.T) {
	m := routeChangerConfig{}
	err := yaml.Unmarshal([]byte(`"#Source[(](.*)[)]#ClientIP($1)#"`), &m)
	if err != nil {
		t.Errorf("Failed to yaml unmarshal: %v", err)
	}
	if len(m) != 1 {
		t.Errorf("Failed to get correct routechanger config: %d != 1", len(m))
	}
}

func TestRouteChangerYamlErr(t *testing.T) {
	m := &routeChangerConfig{}
	err := yaml.Unmarshal([]byte(`foo: foo=bar`), m)
	if err == nil {
		t.Error("Failed to get error on wrong yaml input")
	}
}
