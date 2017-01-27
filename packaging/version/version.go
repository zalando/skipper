package main

import (
	"errors"
	"fmt"
	"log"
	"os"
)

const format = "v%d.%d.%d"

const (
	cmdMajor = "major"
	cmdMinor = "minor"
	cmdPatch = "patch"
)

var invalidCommand = errors.New("invalid command")

func usage() string {
	return `version <major|minor|patch> <currentVersion>`
}

func command() (string, string, error) {
	if len(os.Args) != 3 {
		return "", "", invalidCommand
	}

	switch os.Args[1] {
	case cmdMajor, cmdMinor, cmdPatch:
		return os.Args[1], os.Args[2], nil
	default:
		return "", "", invalidCommand
	}
}

func inc(cmd string, major, minor, patch *int) {
	switch cmd {
	case cmdMajor:
		*major++
		*minor = 0
		*patch = 0
	case cmdMinor:
		*minor++
		*patch = 0
	case cmdPatch:
		*patch++
	}
}

func main() {
	cmd, current, err := command()
	if err != nil {
		log.Fatalln(usage())
	}

	var major, minor, patch int
	_, err = fmt.Sscanf(current, format, &major, &minor, &patch)
	if err != nil {
		log.Fatalln(err)
	}

	inc(cmd, &major, &minor, &patch)
	fmt.Printf(format, major, minor, patch)
}
