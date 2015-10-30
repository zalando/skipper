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

// validate medium from args, and check if it can be used
// as input. Select default etcd, if no medium specified.
// (check and print)
func validateSelectRead(media []*medium) (input, _ *medium, err error) {
	if len(media) > 1 {
		return nil, nil, tooManyInputs
	}

	if len(media) == 0 {
		m, err := processEtcdArgs(defaultEtcdUrls, defaultEtcdPrefix)
		return m, nil, err
	}

	if media[0].typ == inlineIds {
		return nil, nil, invalidInputType
	}

	return media[0], nil, nil
}

// validate media from args, and check if input was specified.
// Select default etcd if no output etcd was specified.
// (upsert, reset, delete)
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
		out, err = processEtcdArgs(defaultEtcdUrls, defaultEtcdPrefix)
		if err != nil {
			return nil, nil, err
		}
	}

	return in, out, nil
}

// Validate media from args for the current command, and select input and/or output.
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
