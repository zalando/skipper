package oauth

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
)

const (
	credentialsDir = "CREDENTIALS_DIR"
	grantType      = "password"
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

type OAuthClient struct {
	oauthUrl         string
	permissionScopes string
	httpClient       *http.Client
}

func Make(oauthUrl, permissionScopes string) *OAuthClient {
	return &OAuthClient{oauthUrl, permissionScopes, &http.Client{}}
}

func (oc *OAuthClient) Token() (string, error) {
	uc, err := getUserCredentials()
	if err != nil {
		return "", err
	}

	cc, err := getClientCredentials()
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

func (oc *OAuthClient) getAuthPostBody(us *userCredentials) string {
	parameters := url.Values{}
	parameters.Add("grant_type", grantType)
	parameters.Add("username", us.Username)
	parameters.Add("password", us.Password)
	parameters.Add("scope", oc.permissionScopes)
	return parameters.Encode()
}

func getCredentials(to interface{}, fn string) error {
	data, err := getCredentialsData(fn)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, to)
}

func getClientCredentials() (*clientCredentials, error) {
	cc := &clientCredentials{}
	err := getCredentials(&cc, "client.json")
	return cc, err
}

func getUserCredentials() (*userCredentials, error) {
	uc := &userCredentials{}
	err := getCredentials(&uc, "user.json")
	return uc, err
}

func getCredentialsData(fn string) ([]byte, error) {
	dir := getCredentialsDir()
	fn = path.Join(dir, fn)
	return ioutil.ReadFile(fn)
}

func getCredentialsDir() string {
	return os.Getenv("CREDENTIALS_DIR")
}
