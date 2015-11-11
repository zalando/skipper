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

package etcd_test

import (
	"github.com/rcrowley/go-metrics"
	"github.com/zalando/skipper/etcd"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/proxy"
	"github.com/zalando/skipper/routing"
)

func Example() {
	// create etcd data client:
	dataClient := etcd.New([]string{"https://etcd.example.org"}, "/skipper")

	// create http.Handler:
	proxy.New(
		routing.New(routing.Options{
			FilterRegistry: builtin.MakeRegistry(),
			DataClients:    []routing.DataClient{dataClient}}),
		proxy.OptionsNone, metrics.NewRegistry())
}
