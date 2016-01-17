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

/*
Package etcdtest implements an easy startup script to start a local etcd
instance for testing purpose.
*/
package etcdtest

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/coreos/etcd/etcdmain"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

var Urls []string

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

func randPort() int {
	return (1 << 15) + rand.Intn(1<<15)
}

// Starts an etcd server.
func Start() error {
	// assuming that the tests won't try to start it concurrently,
	// fix this only when it turns out to be a wrong assumption
	if started {
		return nil
	}

	Urls = makeLocalUrls(randPort(), randPort())
	clientUrlsString := strings.Join(Urls, ",")

	var args []string
	args, os.Args = os.Args, []string{
		"etcd",
		formatFlag("-listen-client-urls", clientUrlsString),
		formatFlag("-advertise-client-urls", clientUrlsString),
		formatFlag("-listen-peer-urls", strings.Join(makeLocalUrls(randPort(), randPort()), ","))}

	go func() {
		// best mock is the real thing
		etcdmain.Main()
	}()

	// wait for started:
	wait := make(chan int)
	go func() {
		for {
			_, err := http.Get(Urls[0] + "/v2/keys")
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

func DeleteAll() error {
	return DeleteAllFrom("/skippertest")
}

func DeleteAllFrom(prefix string) error {
	req, err := http.NewRequest("DELETE", Urls[0]+"/v2/keys"+prefix+"/routes?recursive=true", nil)
	if err != nil {
		return err
	}

	rsp, err := (&http.Client{}).Do(req)
	defer rsp.Body.Close()
	return err
}

func DeleteData(key string) error {
	return DeleteDataFrom("/skippertest", key)
}

func DeleteDataFrom(prefix, key string) error {
	req, err := http.NewRequest("DELETE",
		Urls[0]+"/v2/keys"+prefix+"/routes/"+key,
		nil)
	if err != nil {
		return err
	}
	rsp, err := (&http.Client{}).Do(req)
	if err != nil {
		return err
	}

	defer rsp.Body.Close()
	return nil
}

func PutData(key, data string) error {
	return PutDataTo("/skippertest", key, data)
}

func PutDataTo(prefix, key, data string) error {
	v := make(url.Values)
	v.Add("value", data)
	req, err := http.NewRequest("PUT",
		Urls[0]+"/v2/keys/skippertest/routes/"+key,
		bytes.NewBufferString(v.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rsp, err := (&http.Client{}).Do(req)
	if err != nil {
		return err
	}

	defer rsp.Body.Close()
	return nil
}

func ResetData() error {
	return ResetDataIn("/skippertest")
}

func ResetDataIn(prefix string) error {
	const testRoute = `
		PathRegexp(".*\\.html") ->
		customHeader(3.14) ->
		xSessionId("s4") ->
		"https://www.example.org"
	`

	if err := DeleteAllFrom(prefix); err != nil {
		return err
	}

	return PutDataTo(prefix, "pdp", testRoute)
}

func GetNode(key string) (string, error) {
	return GetNodeFrom("/skippertest", key)
}

func GetNodeFrom(prefix, key string) (string, error) {
	rsp, err := http.Get(Urls[0] + "/v2/keys" + prefix + "/routes/" + key)
	if err != nil {
		return "", err
	}

	defer rsp.Body.Close()

	if rsp.StatusCode < http.StatusOK || rsp.StatusCode >= http.StatusMultipleChoices {
		return "", errors.New("unexpected response status")
	}

	b, err := ioutil.ReadAll(rsp.Body)
	return string(b), err
}
