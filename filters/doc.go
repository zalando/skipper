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

// This package contains specifications for filters.
//
// To create a filterin, create first a subdirectory, conventionally with the name of your
// filter, implement the skipper.FilterSpec and skipper.Filter interfaces, and add the registering call
// to the Register function in filters.go.
//
// For convenience, the noop filter can be composed into the implemented filter, and only the so the
// implementation can shadow only the methods that are relevant ("override").
package filters
