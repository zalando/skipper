package main

type defaultFunc func(in, out *medium) (input, output *medium, err error)

// map command string to defaults
var commandToDefaultMediums = map[command]defaultFunc{
	check:  defaultRead,
	print:  defaultRead,
	upsert: defaultWrite,
	reset:  defaultWrite,
	delete: defaultWrite}

func defaultRead(in, out *medium) (input, output *medium, err error) {
	input = in
	if input == nil {
		// take innkeeper as default
		input, err = processEtcdArgs(defaultEtcdUrls, defaultEtcdPrefix)

		if err != nil {
			return nil, nil, err
		}
	}
	return input, nil, nil
}

func defaultWrite(in, out *medium) (input, output *medium, err error) {
	input = in
	output = out
	if out == nil {
		var err error
		// innkeeper as default
		output, err = processEtcdArgs(defaultEtcdUrls, defaultEtcdPrefix)
		if err != nil {
			return nil, nil, err
		}
	}

	return input, output, nil
}

// selects a default medium for in or out, in case it's needed and not specified
func addDefaultMedia(cmd command, in, out *medium) (input, output *medium, err error) {
	// cmd should be present and valid
	return commandToDefaultMediums[cmd](in, out)
}
