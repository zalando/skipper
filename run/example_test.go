package run_test

import (
	"github.com/zalando/skipper/run"
	"log"
)

func Example() {
	// the file routes.eskip may contain e.g:
	// route1: Path("/some/path") -> "https://api.example.org";
	// route2: Any() -> "https://www.example.org"

	// start skipper listener:
	log.Fatal(run.Run(run.Options{
		Address:    ":8080",
		RoutesFile: "routes.eskip"}))
}
