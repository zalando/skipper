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

package etcd

import (
	"errors"
	"fmt"
	"github.com/coreos/etcd/etcdmain"
	"github.com/coreos/go-etcd/etcd"
	"os"
	"strings"
	"time"
)

const (
	ClientPort1 = 9379
	ClientPort2 = 9401
	PeerPort1   = 9380
	PeerPort2   = 9701
)

var etcdUrls []string

var started bool = false

func makeLocalUrls(ports ...int) []string {
	urls := make([]string, len(ports))
	for i, p := range ports {
		urls[i] = fmt.Sprintf("http://0.0.0.0:%d", p)
	}

	return urls
}

func formatFlag(key, value string) string {
	return fmt.Sprintf("%s=%s", key, value)
}

// starts an etcd server
func startEtcd() error {
	// assuming that the tests won't try to start it concurrently,
	// fix this only when it turns out to be a wrong assumption
	if started {
		return nil
	}

	etcdUrls = makeLocalUrls(ClientPort1, ClientPort2)
	clientUrlsString := strings.Join(etcdUrls, ",")

	var args []string
	args, os.Args = os.Args, []string{
		"etcd",
		formatFlag("-listen-client-urls", clientUrlsString),
		formatFlag("-advertise-client-urls", clientUrlsString),
		formatFlag("-listen-peer-urls", strings.Join(makeLocalUrls(PeerPort1, PeerPort2), ","))}

	go func() {
		// best mock is the real thing
		etcdmain.Main()
	}()

	// wait for started:
	wait := make(chan int)
	go func() {
		for {
			c := etcd.NewClient(etcdUrls)
			_, err := c.Get("/", false, false)

			if err == nil {

				// revert the args for the rest of the tests:
				os.Args = args

				close(wait)
				return
			}

			time.Sleep(30 * time.Millisecond)
		}
	}()

	select {
	case <-wait:
		started = true
		return nil
	case <-time.After(6 * time.Second):
		return errors.New("etcd timeout")
	}
}
