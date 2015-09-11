package oauth

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
)

const credentialsDir = "CREDENTIALS_DIR"

const oauthUrl = "https://auth.zalando.com/oauth2/access_token?realm=/services"

type client struct {
	Id     string `json:"client_id"`
	Secret string `json:"client_secret"`
}

type user struct {
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
	httpClient *http.Client
}

func Make(cl *http.Client) *OAuthClient {
	return &OAuthClient{cl}
}

func (oc *OAuthClient) Authenticate() (string, error) {
	user, err := getUser()
	if err != nil {
		return "", err
	}

	client, err := getClient()
	if err != nil {
		return "", err
	}

	log.Printf("getting a token for username:[%s] clientId:[%s]", user.Username, client.Id)

	postBody := getAuthPostBody(user)

	req, err := http.NewRequest("POST", oauthUrl, strings.NewReader(postBody))

	if err != nil {
		return "", err
	}

	req.SetBasicAuth(client.Id, client.Secret)
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

	json.Unmarshal(authResponseBody, &ar)
	log.Println("token successfully retrieved")

	return ar.AccessToken, nil
}

func getAuthPostBody(us *user) string {
	parameters := url.Values{}
	parameters.Add("grant_type", "password")
	parameters.Add("username", us.Username)
	parameters.Add("password", us.Password)
	// TODO add the real scopes once they are live!
	parameters.Add("scope", "uid")

	postBody := parameters.Encode()

	return postBody
}

func getClient() (*client, error) {
	clientJson, err := getCredentialsJson("client")
	if err != nil {
		return nil, err
	}

	client := &client{}
	if err := json.Unmarshal(clientJson, client); err != nil {
		return nil, err
	}

	return client, nil
}

func getUser() (*user, error) {
	userJson, err := getCredentialsJson("user")
	if err != nil {
		return nil, err
	}

	user := &user{}
	if err := json.Unmarshal(userJson, user); err != nil {
		return nil, err
	}

	return user, nil
}

func getCredentialsJson(file string) ([]byte, error) {
	credentialsDir := getCredentialsDir()

	content, err := ioutil.ReadFile(credentialsDir + "/" + file + ".json")

	if err != nil {
		println(err)
		return nil, err
	}

	log.Println("getting credentials json")

	return content, nil
}

func getCredentialsDir() string {
	return os.Getenv("CREDENTIALS_DIR")
}
