package main

import "os"

const helpHint = "To print eskip usage, enter:\n\neskip help"

func printHint() {
	printStderr(helpHint)
}

func usage() {
	flags.SetOutput(os.Stderr)
	flags.PrintDefaults()
	flags.SetOutput(nowrite)
}

func helpCmd(in, out *medium) error {
	usage()
	return nil
}
