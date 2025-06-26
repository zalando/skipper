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

package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"runtime/debug"
)

type (
	command     string
	commandFunc func(cmdArgs) error
)

const (
	check  command = "check"
	print  command = "print"
	upsert command = "upsert"
	reset  command = "reset"
	delete command = "delete"
	patch  command = "patch"
	ver    command = "version"
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

// map command string to command function
var commands = map[command]commandFunc{
	check:  checkCmd,
	print:  printCmd,
	upsert: upsertCmd,
	reset:  resetCmd,
	delete: deleteCmd,
	patch:  patchCmd,
	ver:    versionCmd}

var (
	errMissingCommand = errors.New("missing command")
	errInvalidCommand = errors.New("invalid command")
)

var stdout io.Writer = os.Stdout

type cmdArgs struct {
	in, out  *medium
	allMedia []*medium
}

func printStderr(args ...interface{}) {
	fmt.Fprintln(os.Stderr, args...)
}

// exit with 0 if no error.
// print error, print hint if set and exit with
// non-0.
func exitErrHint(err error, hint bool) {
	if err == nil {
		os.Exit(0)
	}

	printStderr(err)
	if hint {
		printStderr()
		printStderr(helpHint)
	}

	os.Exit(-1)
}

func exitHint(err error) { exitErrHint(err, true) }
func exit(err error)     { exitErrHint(err, false) }

// second argument must be the ('sub') command.
func getCommand(args []string) (command, error) {
	if len(args) < 2 {
		return "", errMissingCommand
	}

	cmd := command(args[1])
	if cmd[0] == '-' {
		return "", errMissingCommand
	}

	if _, ok := commands[cmd]; ok {
		return cmd, nil
	} else {
		return "", errInvalidCommand
	}
}

func versionCmd(cmdArgs) error {
	fmt.Printf(
		"eskip version %s (commit: %s)\n",
		version, commit,
	)

	return nil
}

func main() {
	// print detailed usage if requested and exit:
	if isHelp() {
		usage()
		exit(nil)
	}

	cmd, err := getCommand(os.Args)
	if err != nil {
		exitHint(err)
	}

	if cmd == ver {
		commands[cmd](cmdArgs{})
		exit(nil)
	}

	// process arguments, not checking if they make any sense:
	media, err := processArgs()
	if err != nil {
		exitHint(err)
	}

	// check if the arguments make sense, and select input/output
	// based on the rules of the current command.
	cmdArgs, err := validateSelectMedia(cmd, media)
	if err != nil {
		exitHint(err)
	}

	cmdArgs, err = addDefaultMedia(cmd, cmdArgs)
	if err != nil {
		exitHint(err)
	}

	// execute command:
	exit(commands[cmd](cmdArgs))
}
