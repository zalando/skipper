// Copyright 2015 Zalando SE
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
