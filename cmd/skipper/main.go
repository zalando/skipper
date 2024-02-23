/*
This command provides an executable version of skipper with the default
set of filters.

For the list of command line options, run:

	skipper -help

For details about the usage and extensibility of skipper, please see the
documentation of the root skipper package.

To see which built-in filters are available, see the skipper/filters
package documentation.
*/
package main

import (
	"fmt"
	"runtime"
	"runtime/debug"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper"
	"github.com/zalando/skipper/config"
)

var (
	version string
	commit  string
)

func init() {
	if info, ok := debug.ReadBuildInfo(); ok {
		if version == "" {
			version = info.Main.Version
		}
		if commit == "" {
			for _, setting := range info.Settings {
				if setting.Key == "vcs.revision" {
					commit = setting.Value[:min(8, len(setting.Value))]
					break
				}
			}
		}
	}
}

func main() {
	cfg := config.NewConfig()
	if err := cfg.Parse(); err != nil {
		log.Fatalf("Error processing config: %s", err)
	}

	if cfg.PrintVersion {
		fmt.Printf("Skipper version %s (", version)
		if commit != "" {
			fmt.Printf("commit: %s, ", commit)
		}
		fmt.Printf("runtime: %s)\n", runtime.Version())
		return
	}

	log.SetLevel(cfg.ApplicationLogLevel)
	if err := skipper.Run(cfg.ToOptions()); err != nil {
		log.Fatal(err)
	}
}
