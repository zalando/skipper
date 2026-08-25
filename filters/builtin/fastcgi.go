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

package builtin

import (
	"github.com/zalando/skipper/filters"
)

type setFastCgiFilenameSpec struct {
	fileName string
}

// NewSetFastCgiFilename returns a filter spec that makes it possible to change
// the FastCGI filename.
func NewSetFastCgiFilename() filters.Spec { return &setFastCgiFilenameSpec{} }

func (s *setFastCgiFilenameSpec) Name() string { return filters.SetFastCgiFilenameName }

func (s *setFastCgiFilenameSpec) CreateFilter(args []any) (filters.Filter, error) {
	if len(args) != 1 {
		return nil, filters.ErrInvalidFilterParameters
	}

	if a, ok := args[0].(string); ok {
		return setFastCgiFilenameSpec{a}, nil
	}

	return nil, filters.ErrInvalidFilterParameters
}

func (s setFastCgiFilenameSpec) Response(_ filters.FilterContext) {}

func (s setFastCgiFilenameSpec) Request(ctx filters.FilterContext) {
	ctx.StateBag()["fastCgiFilename"] = s.fileName
}
