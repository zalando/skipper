// +build !race

package skipper

import "testing"

func TestLoadPlugins(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	o := Options{
		PluginDirs:    []string{"./_test_plugins"},
		FilterPlugins: [][]string{{"filter_noop"}},
	}
	if err := o.findAndLoadPlugins(); err != nil {
		t.Fatalf("Failed to load plugins: %s", err)
	}
}

func TestLoadPluginsFail(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	o := Options{
		PluginDirs: []string{"./_test_plugins_fail"},
	}
	if err := o.findAndLoadPlugins(); err == nil {
		t.Fatalf("did not fail to load plugins: %s", err)
	}
}
