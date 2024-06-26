package config

import (
	"testing"

	"gopkg.in/yaml.v2"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/skipper/eskip"
)

func TestDefaultFiltersFlagsSet(t *testing.T) {
	oneFilter := eskip.MustParseFilters(`tee("https://www.zalando.de/")`)
	manyFilters := eskip.MustParseFilters(`ratelimit(5, "10s") -> tee("https://www.zalando.de/")`)
	manyMoreFilters := eskip.MustParseFilters(`ratelimit(5, "10s") -> tee("https://www.zalando.de/") -> inlineContent("OK\n")`)

	tests := []struct {
		name       string
		args       []string
		yaml       string
		want       []*eskip.Filter
		wantString string
	}{
		{
			name:       "test no filter",
			args:       nil,
			want:       nil,
			wantString: "",
		},
		{
			name:       "test empty filter",
			args:       []string{""},
			yaml:       "",
			want:       nil,
			wantString: "",
		},
		{
			name:       "test one filter",
			args:       []string{`tee("https://www.zalando.de/")`},
			yaml:       `field: tee("https://www.zalando.de/")`,
			want:       oneFilter,
			wantString: `tee("https://www.zalando.de/")`,
		},
		{
			name:       "test many filters in one value",
			args:       []string{`ratelimit(5, "10s") -> tee("https://www.zalando.de/")`},
			yaml:       `field: ratelimit(5, "10s") -> tee("https://www.zalando.de/")`,
			want:       manyFilters,
			wantString: `ratelimit(5, "10s") -> tee("https://www.zalando.de/")`,
		},
		{
			name: "test multiple values",
			args: []string{`ratelimit(5, "10s") -> tee("https://www.zalando.de/")`, `inlineContent("OK\n")`},
			yaml: `
field:
  - ratelimit(5, "10s") -> tee("https://www.zalando.de/")
  - inlineContent("OK\n")
`,
			want:       manyMoreFilters,
			wantString: `ratelimit(5, "10s") -> tee("https://www.zalando.de/") -> inlineContent("OK\n")`,
		},
		{
			name: "test multiple with empty filters in between",
			args: []string{
				`    `, // whitespaces only
				`ratelimit(5, "10s")`,
				`// not a filter, just an eskip comment`,
				`tee("https://www.zalando.de/")`,
				``, // empty
			},
			yaml: `
field:
  - '    ' # whitespaces only
  - ratelimit(5, "10s")
  - // not a filter, just an eskip comment
  - tee("https://www.zalando.de/")
  - '' # empty
`,
			want:       manyFilters,
			wantString: `ratelimit(5, "10s") -> tee("https://www.zalando.de/")`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := struct {
				Field *defaultFiltersFlags
			}{
				Field: &defaultFiltersFlags{},
			}

			var err error
			for _, arg := range tt.args {
				err = cfg.Field.Set(arg)
				if err != nil {
					break
				}
			}
			require.NoError(t, err)

			assert.Equal(t, tt.wantString, cfg.Field.String())
			assert.Equal(t, tt.want, cfg.Field.filters)

			if tt.yaml != "" {
				err := yaml.Unmarshal([]byte(tt.yaml), &cfg)
				require.NoError(t, err)

				assert.Equal(t, tt.want, cfg.Field.filters)
			}
		})
	}
}

func TestDefaultFiltersFlagsInvalidFlag(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{
			name: "test single filter",
			args: []string{"invalid-filter"},
		},
		{
			name: "test multiple filters",
			args: []string{`status(204)`, "invalid-filter"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := struct {
				Field *defaultFiltersFlags
			}{
				Field: &defaultFiltersFlags{},
			}

			var err error
			for _, arg := range tt.args {
				err = cfg.Field.Set(arg)
				if err != nil {
					break
				}
			}
			assert.Error(t, err)
		})
	}
}

func TestDefaultFiltersFlagsInvalidYaml(t *testing.T) {
	tests := []struct {
		name string
		yaml string
	}{
		{
			name: "test one value",
			yaml: `field: invalid-filter`,
		},
		{
			name: "test multiple values",
			yaml: `
field:
  - ratelimit(5, "10s") -> tee("https://www.zalando.de/")
  - invalid-filter
	`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := struct {
				Field *defaultFiltersFlags
			}{
				Field: &defaultFiltersFlags{},
			}

			err := yaml.Unmarshal([]byte(tt.yaml), &cfg)
			require.Error(t, err)
		})
	}
}
