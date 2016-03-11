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
	validateSelectFunc func([]*medium) (cmdArgs, error)
)

const (
	none mediaType = iota
	stdin
	file
	etcd
	innkeeper
	inline
	inlineIds
	patchPrepend
	patchPrependFile
	patchAppend
	patchAppendFile
)

var commandToValidations = map[command]validateSelectFunc{
	check:  validateSelectRead,
	print:  validateSelectRead,
	upsert: validateSelectWrite,
	reset:  validateSelectWrite,
	delete: validateSelectDelete,
	patch:  validateSelectPatch}

type medium struct {
	typ          mediaType
	urls         []*url.URL
	path         string
	eskip        string
	ids          []string
	oauthToken   string
	patchFilters string
	patchFile    string
}

var (
	tooManyInputs    = errors.New("too many inputs")
	invalidInputType = errors.New("invalid input type")
	missingInput     = errors.New("missing input")
)

// validate medium from args, and check if it can be used
// as input.
// (check and print)
func validateSelectRead(media []*medium) (a cmdArgs, err error) {
	if len(media) > 1 {
		err = tooManyInputs
		return
	}

	if len(media) == 0 {
		a.in, err = processEtcdArgs(defaultEtcdUrls, defaultEtcdPrefix)
		return
	}

	switch media[0].typ {
	case inlineIds, patchPrepend, patchPrependFile, patchAppend, patchAppendFile:
		err = invalidInputType
		return
	}

	a.in = media[0]
	return
}

// validate media from args, and check if input was specified.
func validateSelectWrite(media []*medium) (a cmdArgs, err error) {
	if len(media) == 0 {
		err = missingInput
		return
	}

	if len(media) > 2 {
		err = tooManyInputs
		return
	}

	for _, m := range media {
		switch media[0].typ {
		case inlineIds, patchPrepend, patchPrependFile, patchAppend, patchAppendFile:
			err = invalidInputType
			return
		}

		if m.typ == etcd || m.typ == innkeeper {
			a.out = m
		} else {
			a.in = m
		}
	}

	if a.in == nil {
		err = missingInput
	}

	return
}

func validateSelectDelete(media []*medium) (a cmdArgs, err error) {
	if len(media) == 0 {
		err = missingInput
		return
	}

	if len(media) > 2 {
		err = tooManyInputs
		return
	}

	for _, m := range media {
		switch media[0].typ {
		case patchPrepend, patchPrependFile, patchAppend, patchAppendFile:
			err = invalidInputType
			return
		}

		if m.typ == etcd || m.typ == innkeeper {
			a.out = m
		} else {
			a.in = m
		}
	}

	if a.in == nil {
		err = missingInput
	}

	return
}

func validateSelectPatch(media []*medium) (a cmdArgs, err error) {
	for _, m := range media {
		switch m.typ {
		case patchPrepend, patchPrependFile, patchAppend, patchAppendFile:
		case inlineIds:
			err = invalidInputType
			return
		default:
			if a.in != nil {
				err = tooManyInputs
				return
			}

			a.in = m
		}
	}

	return
}

// Validates media from args for the current command, and selects input and/or output.
func validateSelectMedia(cmd command, media []*medium) (cmdArgs cmdArgs, err error) {
	a, err := commandToValidations[cmd](media)
	a.allMedia = media
	return a, err
}
