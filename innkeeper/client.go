// Package innkeeper implements a data client for the Innkeeper API.
package innkeeper

import (
	"encoding/json"
	"fmt"
	"github.com/zalando/skipper/skipper"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"time"
	"crypto/tls"
	"io"
	"errors"
)


const authFailedMessage = "Authentication failed"

// serialization object for innkeeper data
//
// todo
//
type routeData struct {
	Id      int64  `json:"id"`
	Deleted bool   `json:"deleted"`
	Route   string `json:"route"`
}

type apiError struct {
	Message string `json:"message"`
}

type routeDoc string

type client struct {
	pollUrl    string
	httpClient *http.Client
	dataChan   chan skipper.RawData
	authToken  string

	// store the routes for comparison during
	// the subsequent polls
	doc map[int64]string
}

// Creates an innkeeper client.
func Make(pollUrl string, pollTimeout time.Duration) skipper.DataClient {

	tr := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
	c := &client{
		pollUrl,
		&http.Client{Transport: tr},
		make(chan skipper.RawData),"",
		make(map[int64]string)}

	// start a polling loop
	go func() {
		for {
			if c.poll() {
				c.dataChan <- toDocument(c.doc)
			}

			time.Sleep(pollTimeout)
		}
	}()

	return c
}

func parseApiError(r io.Reader) (string, error) {
	message, err := ioutil.ReadAll(r)

	if err != nil {
		return "", err
	}

	ae := apiError{}
	if err := json.Unmarshal(message, &ae); err != nil {
		return "", err
	}

	return ae.Message, nil
}

func (c *client) authenticate() error {
	c.authToken = "99daaa44-2e63-4bb1-a9e1-47099ca6c930"
	return nil
}


// makes a request to innkeeper for the latest data
func (c *client) getData(retry bool) ([]*routeData, error) {

	req, err := http.NewRequest("GET", c.pollUrl, nil)

	if err != nil {
		return nil, err
	}
	req.Header.Add("Authorization", c.authToken)

	response, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	defer response.Body.Close()

	if response.StatusCode == 401 {
		message, err := parseApiError(response.Body)

		if err != nil {
			return nil, err
		}

		if message == authFailedMessage && retry {
			err := c.authenticate()
			if err != nil {
				return nil, err
			}
			return c.getData(false)
		}

		return nil, errors.New("unknown auth error")
	}

	if response.StatusCode >= 400 {
		return nil, fmt.Errorf("failed to receive data: %s", response.Status)
	}

	data, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}

	println(string(data))

	var parsed []*routeData
	err = json.Unmarshal(data, &parsed)
	return parsed, err
}

// updates the route doc from received data, and
// returns true if there were any changes, otherwise
// false.
//
// todo
//
func updateDoc(doc map[int64]string, data []*routeData) bool {
	changed := false
	for _, dataItem := range data {
		route, exists := doc[dataItem.Id]
		switch {
		case exists && dataItem.Deleted:
			delete(doc, dataItem.Id)
			changed = true
		case (exists && route != dataItem.Route) || (!exists && !dataItem.Deleted):
			doc[dataItem.Id] = dataItem.Route
			changed = true
		}
	}

	return changed
}

// polls the innkeeper API, and updates the route doc
// if there were any changes. If yes, then returns
// true, otherwise false.
func (c *client) poll() bool {
	data, err := c.getData(true)
	if err == nil {
		return updateDoc(c.doc, data)
	} else {
		log.Println("error while receiving innkeeper data;", err)
		return false
	}
}

// returns eskip format
func toDocument(doc map[int64]string) routeDoc {
	var routeDefs []string
	for k, r := range doc {
		routeDefs = append(routeDefs, fmt.Sprintf("route%d: %s", uint64(k), r))
	}

	return routeDoc(strings.Join(routeDefs, ";"))
}

// returns skipper raw data value
func (c *client) Receive() <-chan skipper.RawData { return c.dataChan }

// returns eskip format of the route doc
//
// todo: since only the routes are stored in the
// RawData interface, no need for it, it can be
// just a string
func (d routeDoc) Get() string { return string(d) }
