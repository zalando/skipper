package main

type defaultFunc func(cmdArgs) (cmdArgs, error)

// map command string to defaults
var commandToDefaultMediums = map[command]defaultFunc{
	check:  defaultRead,
	print:  defaultRead,
	upsert: defaultWrite,
	reset:  defaultWrite,
	delete: defaultWrite,
	patch:  defaultRead}

func defaultRead(a cmdArgs) (aa cmdArgs, err error) {
	aa = a
	if aa.in == nil {
		aa.in, err = processEtcdArgs(defaultEtcdUrls, defaultEtcdPrefix, "")
	}
	return
}

func defaultWrite(a cmdArgs) (aa cmdArgs, err error) {
	aa = a
	if aa.out == nil {
		aa.out, err = processEtcdArgs(defaultEtcdUrls, defaultEtcdPrefix, "")
	}

	return
}

// selects a default medium for in or out, when it's needed and not specified
func addDefaultMedia(cmd command, a cmdArgs) (cmdArgs, error) {
	return commandToDefaultMediums[cmd](a)
}
