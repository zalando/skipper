/*
Package etcdtest implements an easy startup script to start a local etcd
instance for testing purpose.
*/
package etcdtest

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var Urls []string

var etcd *exec.Cmd
var etcdDataDir string

func makeLocalUrls(ports ...int) []string {
	urls := make([]string, len(ports))
	for i, p := range ports {
		urls[i] = fmt.Sprintf("http://0.0.0.0:%d", p)
	}

	return urls
}

func randPort() int {
	return (1 << 15) + rand.Intn(1<<15) // #nosec
}

// Starts an etcd server.
func Start() error {
	return StartProjectRoot("")
}

// StartProjectRoot starts an etcd server. If projectRoot is not empty, then it checks
// if the .bin/etcd binary exists, and uses that instead of the one in the path.
func StartProjectRoot(projectRoot string) error {
	// assuming that the tests won't try to start it concurrently,
	// fix this only when it turns out to be a wrong assumption
	if etcd != nil {
		return nil
	}

	Urls = makeLocalUrls(randPort(), randPort())
	clientUrlsString := strings.Join(Urls, ",")

	dir, err := os.MkdirTemp("", "etcdtest")
	if err != nil {
		return err
	}
	etcdDataDir = dir

	var binary string
	if projectRoot != "" {
		binary = filepath.Join(projectRoot, ".bin/etcd")
		_, err := os.Stat(binary)
		if os.IsNotExist(err) {
			binary = ""
		}
	}

	if binary == "" {
		binary = "etcd"
	}

	/* #nosec */
	e := exec.Command(binary,
		"-data-dir", etcdDataDir,
		"-listen-client-urls", clientUrlsString,
		"-advertise-client-urls", clientUrlsString)
	stderr, err := e.StderrPipe()
	if err != nil {
		return err
	}
	stdout, err := e.StdoutPipe()
	if err != nil {
		return err
	}
	err = e.Start()
	if err != nil {
		return err
	}

	// wait for started:
	wait := make(chan int)
	go func() {
		for {
			rsp, err := http.Get(Urls[0] + "/v2/keys")
			if err == nil {
				rsp.Body.Close()
				close(wait)
				return
			}

			time.Sleep(30 * time.Millisecond)
		}
	}()

	select {
	case <-wait:
		etcd = e
		return nil
	case <-time.After(6 * time.Second):
		bout, _ := io.ReadAll(stdout)
		berr, _ := io.ReadAll(stderr)
		log.Panicf("ETCD timeout: Failed to start etcd\netcd log output\nSTDOUT: %s\nSTDERR: %s", string(bout), string(berr))
		return fmt.Errorf("etcd timeout")
	}
}

func Stop() error {
	if etcd == nil {
		return nil
	}

	defer func() {
		os.RemoveAll(etcdDataDir)
		etcdDataDir = ""
	}()

	return etcd.Process.Kill()
}

// Deletes the 'routes' directory from etcd with the prefix '/skippertest'.
func DeleteAll() error {
	return DeleteAllFrom("/skippertest")
}

// Deletes the 'routes' directory with the specified prefix.
func DeleteAllFrom(prefix string) error {
	req, err := http.NewRequest("DELETE", Urls[0]+"/v2/keys"+prefix+"/routes?recursive=true", nil)
	if err != nil {
		return err
	}

	rsp, err := (&http.Client{}).Do(req)
	if err != nil {
		return err
	}

	rsp.Body.Close()
	return nil
}

// Deletes a route from etcd with the prefix '/skippertest'.
func DeleteData(key string) error {
	return DeleteDataFrom("/skippertest", key)
}

// Deletes a route from etcd with the specified prefix.
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

// Saves a route in etcd with the prefix '/skippertest'.
func PutData(key, data string) error {
	return PutDataTo("/skippertest", key, data)
}

// Saves a route in etcd with the specified prefix.
func PutDataTo(prefix, key, data string) error {
	return PutDataToTTL(prefix, key, data, 0)
}

// Saves a route with TTL in etcd with the specified prefix.
func PutDataToTTL(prefix, key, data string, ttl int) error {
	v := make(url.Values)
	v.Add("value", data)
	if ttl > 0 {
		v.Add("ttl", strconv.Itoa(ttl))
	}
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

// Deletes all routes in etcd and creates a test route under
// the prefix '/skippertest'.
func ResetData() error {
	return ResetDataIn("/skippertest")
}

// Deletes all routes in etcd and creates a test route under
// the specified prefix.
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

// Loads an etcd route node from the prefix '/skippertest'.
func GetNode(key string) (string, error) {
	return GetNodeFrom("/skippertest", key)
}

// Loads an etcd route node from the specified prefix.
func GetNodeFrom(prefix, key string) (string, error) {
	rsp, err := http.Get(Urls[0] + "/v2/keys" + prefix + "/routes/" + key)
	if err != nil {
		return "", err
	}

	defer rsp.Body.Close()

	if rsp.StatusCode < http.StatusOK || rsp.StatusCode >= http.StatusMultipleChoices {
		return "", errors.New("unexpected response status")
	}

	b, err := io.ReadAll(rsp.Body)
	return string(b), err
}
