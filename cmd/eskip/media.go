package main

import (
	"errors"
	"net/url"
)

type mediaType int

const (
	none mediaType = iota
	stdin
	file
	etcd
	inline
	inlineIds
)

type medium struct {
	typ   mediaType
	urls  []*url.URL
	path  string
	eskip string
	ids   []string
}

var (
	tooManyInputs    = errors.New("too many inputs")
	invalidInputType = errors.New("invalid input type")
	missingInput     = errors.New("missing input")
)

func validateSelectRead(media []*medium) (input, output *medium, err error) {
	if len(media) > 1 {
		return nil, nil, tooManyInputs
	}

	if len(media) == 0 {
		m, err := processEtcdArgs(defaultEtcdUrls, defaultEtcdStorageRoot)
		return m, nil, err
	}

	if media[0].typ == inlineIds {
		return nil, nil, invalidInputType
	}

	return media[0], nil, nil
}

func validateSelectWrite(cmd command, media []*medium) (input, output *medium, err error) {
	if len(media) == 0 {
		return nil, nil, missingInput
	}

	if len(media) > 2 {
		return nil, nil, tooManyInputs
	}

	var in, out *medium
	for _, m := range media {
		if m.typ == inlineIds && cmd != delete {
			return nil, nil, invalidInputType
		}

		if m.typ == etcd {
			out = m
		} else {
			in = m
		}
	}

	if in == nil {
		return nil, nil, missingInput
	}

	if out == nil {
		var err error
		out, err = processEtcdArgs(defaultEtcdUrls, defaultEtcdStorageRoot)
		if err != nil {
			return nil, nil, err
		}
	}

	return in, out, nil
}

func validateSelectMedia(cmd command, media []*medium) (input, output *medium, err error) {
	switch cmd {
	case check, print:
		return validateSelectRead(media)
	case upsert, reset, delete:
		return validateSelectWrite(cmd, media)
	default:
		return nil, nil, invalidCommand
	}
}
