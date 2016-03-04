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
	"net/url"
)

type (
	mediaType          int
	validateSelectFunc func(media []*medium) (in, out *medium, err error)
)

const (
	none mediaType = iota
	stdin
	file
	etcd
	innkeeper
	inline
	inlineIds
)

var commandToValidations = map[command]validateSelectFunc{
	check:  validateSelectRead,
	print:  validateSelectRead,
	upsert: validateSelectWrite,
	reset:  validateSelectWrite,
	delete: validateSelectDelete}

type medium struct {
	typ        mediaType
	urls       []*url.URL
	path       string
	eskip      string
	ids        []string
	oauthToken string
}

var (
	tooManyInputs    = errors.New("too many inputs")
	invalidInputType = errors.New("invalid input type")
	missingInput     = errors.New("missing input")
)

// validate medium from args, and check if it can be used
// as input.
// (check and print)
func validateSelectRead(media []*medium) (input, _ *medium, err error) {
	if len(media) > 1 {
		return nil, nil, tooManyInputs
	}

	if len(media) == 0 {
		m, err := processEtcdArgs(defaultEtcdUrls, defaultEtcdPrefix, "")
		return m, nil, err
	}

	if media[0].typ == inlineIds {
		return nil, nil, invalidInputType
	}

	return media[0], nil, nil
}

// validate media from args, and check if input was specified.
func validateSelectWrite(media []*medium) (input, output *medium, err error) {
	if len(media) == 0 {
		return nil, nil, missingInput
	}

	if len(media) > 2 {
		return nil, nil, tooManyInputs
	}

	var in, out *medium
	for _, m := range media {
		if m.typ == inlineIds {
			return nil, nil, invalidInputType
		}

		if m.typ == etcd || m.typ == innkeeper {
			out = m
		} else {
			in = m
		}
	}

	if in == nil {
		return nil, nil, missingInput
	}

	return in, out, nil
}

func validateSelectDelete(media []*medium) (in, out *medium, err error) {
	if len(media) == 0 {
		return nil, nil, nil
	}

	if len(media) > 2 {
		return nil, nil, tooManyInputs
	}

	for _, m := range media {

		if m.typ == etcd || m.typ == innkeeper {
			out = m
		} else {
			in = m
		}
	}

	if in == nil {
		return nil, nil, missingInput
	}

	return in, out, nil
}

// Validates media from args for the current command, and selects input and/or output.
func validateSelectMedia(cmd command, media []*medium) (input, output *medium, err error) {
	// cmd should be present and valid
	return commandToValidations[cmd](media)
}
