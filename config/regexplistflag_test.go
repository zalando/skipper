package config

import (
	"testing"

	"gopkg.in/yaml.v2"
)

func TestRegexpListFlag(t *testing.T) {
	for _, test := range []struct {
		title     string
		args      []string
		expect    string
		expectErr bool
	}{{
		title:  "cool",
		args:   []string{"foo", "bar"},
		expect: "foo\nbar",
	}, {
		title:     "fail",
		args:      []string{"[", "bar"},
		expectErr: true,
	}} {
		t.Run(test.title, func(t *testing.T) {
			var f regexpListFlag

			var err error
			for _, arg := range test.args {
				if err = (&f).Set(arg); err != nil {
					break
				}
			}

			if err != nil {
				if !test.expectErr {
					t.Fatal(err)
				}

				return
			}

			if test.expectErr {
				t.Fatal("failed to fail")
			}

			s := f.String()
			if s != test.expect {
				t.Log("failed to parse args")
				t.Log("expected:", test.expect)
				t.Log("got:     ", s)
				t.Fatal()
			}
		})
	}
}

func TestRegexpListConfig(t *testing.T) {
	for _, test := range []struct {
		title     string
		config    string
		expect    string
		expectErr bool
	}{{
		title:  "cool",
		config: "kubernetes-allowed-external-names:\n- foo\n- bar",
		expect: "foo\nbar",
	}, {
		title:     "fail",
		config:    "kubernetes-allowed-external-names:\n- [\n- bar",
		expectErr: true,
	}} {
		t.Run(test.title, func(t *testing.T) {
			var config regexpListFlag
			if err := yaml.Unmarshal([]byte(test.config), &config); err != nil {
				if test.expectErr {
					return
				}

				t.Fatal(err)
			}

			if test.expectErr {
				t.Fatal("failed to fail")
			}

			s := config.String()
			if s != test.expect {
				t.Log("failed to parse config")
				t.Log("expected:", test.expect)
				t.Log("got:     ", s)
				t.Fatal()
			}
		})
	}
}
