/*
Package etcdtest implements test utilities to start a local etcd
instance using testcontainers for testing purposes.
*/
package etcdtest

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/docker/go-connections/nat"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

var (
	Urls      []string
	mu        sync.Mutex
	container testcontainers.Container
)

const (
	etcdImage = "gcr.io/etcd-development/etcd:v3.5.18"
	etcdPort  = "2379/tcp"
)

// Start starts an etcd testcontainer with v2 API enabled.
func Start() error {
	mu.Lock()
	defer mu.Unlock()

	if container != nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()

	port, err := nat.NewPort("tcp", "2379")
	if err != nil {
		return fmt.Errorf("invalid port: %w", err)
	}

	c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image: etcdImage,
			Cmd: []string{
				"/usr/local/bin/etcd",
				"--enable-v2=true",
				"--listen-client-urls=http://0.0.0.0:2379",
				"--advertise-client-urls=http://0.0.0.0:2379",
			},
			ExposedPorts: []string{etcdPort},
			WaitingFor: wait.ForAll(
				wait.ForHTTP("/v2/keys").
					WithPort(port.Port()).
					WithStatusCodeMatcher(func(status int) bool {
						return status == http.StatusOK
					}),
				wait.NewHostPortStrategy(port.Port()),
			),
		},
		Started: true,
	})
	if err != nil {
		return fmt.Errorf("failed to start etcd container: %w", err)
	}

	endpointCtx, endpointCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer endpointCancel()

	endpoint, err := c.Endpoint(endpointCtx, "")
	if err != nil {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_ = c.Terminate(cleanupCtx)
		return fmt.Errorf("failed to get etcd endpoint: %w", err)
	}

	Urls = []string{"http://" + endpoint}
	container = c
	return nil
}

// Stop terminates the running etcd container.
func Stop() error {
	mu.Lock()
	defer mu.Unlock()

	if container == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := container.Terminate(ctx)
	container = nil
	Urls = nil
	return err
}

// StartProjectRoot starts an etcd server. Deprecated: delegates to Start.
func StartProjectRoot(_ string) error {
	return Start()
}

// DeleteAll deletes the 'routes' directory from etcd with the prefix '/skippertest'.
func DeleteAll() error {
	return DeleteAllFrom("/skippertest")
}

// DeleteAllFrom deletes the 'routes' directory with the specified prefix.
func DeleteAllFrom(prefix string) error {
	if len(Urls) == 0 {
		return errors.New("etcd container not running")
	}
	req, err := http.NewRequest("DELETE", Urls[0]+"/v2/keys"+prefix+"/routes?recursive=true", nil)
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

// DeleteData deletes a route from etcd with the prefix '/skippertest'.
func DeleteData(key string) error {
	return DeleteDataFrom("/skippertest", key)
}

// DeleteDataFrom deletes a route from etcd with the specified prefix.
func DeleteDataFrom(prefix, key string) error {
	if len(Urls) == 0 {
		return errors.New("etcd container not running")
	}
	req, err := http.NewRequest("DELETE", Urls[0]+"/v2/keys"+prefix+"/routes/"+key, nil)
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

// PutData saves a route in etcd with the prefix '/skippertest'.
func PutData(key, data string) error {
	return PutDataTo("/skippertest", key, data)
}

// PutDataTo saves a route in etcd with the specified prefix.
func PutDataTo(prefix, key, data string) error {
	return PutDataToTTL(prefix, key, data, 0)
}

// PutDataToTTL saves a route with TTL in etcd with the specified prefix.
func PutDataToTTL(prefix, key, data string, ttl int) error {
	if len(Urls) == 0 {
		return errors.New("etcd container not running")
	}
	v := make(url.Values)
	v.Add("value", data)
	if ttl > 0 {
		v.Add("ttl", strconv.Itoa(ttl))
	}
	req, err := http.NewRequest("PUT", Urls[0]+"/v2/keys"+prefix+"/routes/"+key, bytes.NewBufferString(v.Encode()))
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

// ResetData deletes all routes and creates a test route under '/skippertest'.
func ResetData() error {
	return ResetDataIn("/skippertest")
}

// ResetDataIn deletes all routes and creates a test route under the specified prefix.
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

// GetNode loads an etcd route node from the prefix '/skippertest'.
func GetNode(key string) (string, error) {
	return GetNodeFrom("/skippertest", key)
}

// GetNodeFrom loads an etcd route node from the specified prefix.
func GetNodeFrom(prefix, key string) (string, error) {
	if len(Urls) == 0 {
		return "", errors.New("etcd container not running")
	}
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
