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

/*
This utility can be used to verify, print, update or delete skipper
routes in eskip format, from/to different media.

For command line help, enter:

    eskip -help

Examples

Check if an eskip file has valid syntax:

    eskip check routes.eskip

Print routes stored in etcd:

    eskip print -etcd-urls https://etcd.example.org

Insert/update routes in etcd from an eskip file:

    eskip upsert routes.eskip

Sync routes from an eskip file to etcd:

    eskip reset routes.eskip

Delete routes from etcd:

    eskip delete -ids route1,route2,route3

Delete all routes from etcd:

    eskip print | eskip delete

Copy all routes in etcd under a different prefix:

    eskip print | eskip upsert -etcd-prefix /skipper-backup

(Where -etcd-urls is not set for write operations like upsert, reset and
delete, the default etcd cluster urls are used:
http://127.0.0.1:2379,http://127.0.0.1:4001)
*/
package main

import (
	"fmt"
	"os"
)

const (
	// short hint:
	helpHint = "To print eskip usage, enter: eskip -help"

	// flag usage strings:
	etcdUrlsUsage     = "urls of nodes in an etcd cluster"
	etcdPrefixUsage   = "path prefix for routes in etcd"
	innkeeperUrlUsage = "url for the innkeeper service"
	oauthTokenUsage   = "oauth token used to authenticate to innkeeper"
	inlineRoutesUsage = "inline: routes in eskip format"
	inlineIdsUsage    = "inline ids: comma separated route ids"

	// command line help (1):
	help1 = `Usage: eskip <command> [media flags] [--] [file]
Commands: check|print|upsert|reset|delete
Verify, print, update or delete skipper routes.
See more: https://github.com/zalando/skipper

Media types:

innkeeper     endpoint of an innkeeper server. See more about innkeeper:
              https://github.com/zalando/innkeeper
etcd          endpoint(s) of an etcd cluster. See more about etcd:
              https://github.com/coreos/etcd
stdin         standard input when not tty, expecting routes
file          a file containing routes
inline        routes as command line parameter
inline ids    a list of route ids (only for delete)

Media flags:
`

	/* position for generated flags */

	// command line help (2):
	help2 = `
Commands:

check    verifies the syntax of routes. Accepts one input medium
         of the following types: etcd (default), stdin, file, inline.
         Example:
         eskip check -etcd-urls http://etcd.example.org

print    same as check, but also prints the routes.

upsert   insert/update routes from input to output. Expects one input
         medium of the following types: stdin, file, inline.
         Automatically selects etcd as output. Example:
         eskip upsert routes.eskip

reset    same as upsert, but also deletes the routes from the output
         that are not found in the input.

delete   deletes routes from the output that are specified in the input.
         Expects one input medium of the following types: stdin, file,
         inline, inline ids. Automatically selects etcd as output.
         Example:
         eskip delete -ids route1,route2,route3`
)

// simplified check for help request:
func isHelp() bool {
	for _, s := range os.Args[1:] {
		if s == "-help" || s == "--help" {
			return true
		}
	}

	return false
}

// print command line help:
func usage() {
	fmt.Println(help1)
	flags.SetOutput(os.Stdout)
	flags.PrintDefaults()
	flags.SetOutput(nowrite)
	fmt.Println(help2)
}
