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
Package oauth implements an authentication client to be used with OAuth2
authentication services.

The package uses two json documents to retrieve the credentials, with the
file names: client.json and user.json. These documents must be found on the
file system under the directory passed in with the credentialsDir argument.

The structure of the client credentials document:

    {"client_id": "testclientid", "client_secret": "testsecret"}

The structure of the user credentials document:

    {"application_username": "testusername", "application_password": "testpassword"}

The GetToken method ignores the expiration date and makes a new request to the
OAuth2 service on every call, so storing the token, if necessary, is the
responsibility of the calling code.
*/
package oauth

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"
	"strings"
)

const (
	grantType    = "password"
	clientJsonFn = "client.json"
	userJsonFn   = "user.json"
)

type clientCredentials struct {
	Id     string `json:"client_id"`
	Secret string `json:"client_secret"`
}

type userCredentials struct {
	Username string `json:"application_username"`
	Password string `json:"application_password"`
}

type authResponse struct {
	Scope       string `json:"scope"`
	ExpiresIn   int32  `json:"expires_in"`
	TokenType   string `json:"token_type"`
	AccessToken string `json:"access_token"`
}

// An OAuthClient implements authentication to an OAuth2 service.
type OAuthClient struct {
	credentialsDir   string
	oauthUrl         string
	permissionScopes string
	httpClient       *http.Client
}

// Initializes a new OAuthClient.
func New(credentialsDir, oauthUrl, permissionScopes string) *OAuthClient {
	return &OAuthClient{credentialsDir, oauthUrl, permissionScopes, &http.Client{}}
}

// Returns a new authentication token.
func (oc *OAuthClient) GetToken() (string, error) {
	uc, err := oc.getUserCredentials()
	if err != nil {
		return "", err
	}

	cc, err := oc.getClientCredentials()
	if err != nil {
		return "", err
	}

	postBody := oc.getAuthPostBody(uc)
	req, err := http.NewRequest("POST", oc.oauthUrl, strings.NewReader(postBody))
	if err != nil {
		return "", err
	}

	req.SetBasicAuth(cc.Id, cc.Secret)
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	response, err := oc.httpClient.Do(req)
	if err != nil {
		return "", err
	}

	defer response.Body.Close()

	authResponseBody, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return "", err
	}

	var ar *authResponse
	err = json.Unmarshal(authResponseBody, &ar)
	if err != nil {
		return "", err
	}

	return ar.AccessToken, nil
}

// Prepares the POST body of the authentication request.
func (oc *OAuthClient) getAuthPostBody(us *userCredentials) string {
	parameters := url.Values{}
	parameters.Add("grant_type", grantType)
	parameters.Add("username", us.Username)
	parameters.Add("password", us.Password)
	parameters.Add("scope", oc.permissionScopes)
	return parameters.Encode()
}

// Loads and parses the credentials from a credentials document.
func (oc *OAuthClient) getCredentials(to interface{}, fn string) error {
	data, err := ioutil.ReadFile(path.Join(oc.credentialsDir, fn))
	if err != nil {
		return err
	}

	return json.Unmarshal(data, to)
}

// Loads and parses the client credentials.
func (oc *OAuthClient) getClientCredentials() (*clientCredentials, error) {
	cc := &clientCredentials{}
	err := oc.getCredentials(&cc, clientJsonFn)
	return cc, err
}

// Loads and parses the user credentials.
func (oc *OAuthClient) getUserCredentials() (*userCredentials, error) {
	uc := &userCredentials{}
	err := oc.getCredentials(&uc, userJsonFn)
	return uc, err
}
