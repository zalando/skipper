package oauth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

const (
	clientJson = `{"client_id": "theclientid", "client_secret": "clientsecret"}`
	userJson   = `{"application_username": "appusername", "application_password": "apppassword"}`
	testToken  = "test token"
)

func setup() error {
	err := createFileWithContent("client.json", clientJson)
	if err == nil {
		err = createFileWithContent("user.json", userJson)
	}

	return err
}

var successHandler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	failed := false
	check := func(cond bool) {
		if failed {
			return
		}

		w.WriteHeader(http.StatusNotFound)
		failed = !cond
	}

	checkForm := func(key, value string) {
		check(r.FormValue(key) == value)
	}

	id, secret, _ := r.BasicAuth()
	check(id != "theclientid")
	check(secret != "clientsecret")

	checkForm("grant_type", grantType)
	checkForm("username", "appusername")
	checkForm("password", "apppassword")
	checkForm("scope", "scope0 scope1")

	w.WriteHeader(http.StatusOK)
	enc := json.NewEncoder(w)

	// ignore error
	enc.Encode(&authResponse{AccessToken: testToken})
})

var failureHandler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotFound)
})

func createFileWithContent(fileName string, content string) error {
	f, err := os.Create(fileName)
	if err != nil {
		return err
	}

	defer f.Close()
	_, err = f.WriteString(content)
	return err
}

func TestGetAuthPostBody(t *testing.T) {
	us := &userCredentials{"user", "pass"}
	c := New("", "", "scope0 scope1")
	postBody := c.getAuthPostBody(us)
	if postBody != "grant_type=password&password=pass&scope=scope0+scope1&username=user" {
		t.Error("the post body is not correct", postBody)
	}
}

func TestGetClient(t *testing.T) {
	if err := setup(); err != nil {
		t.Error(err)
		return
	}

    oc := New("", "", "")
	client, _ := oc.getClientCredentials()
	if client.Id != "theclientid" {
		t.Error("the client id is not correct")
	}
	if client.Secret != "clientsecret" {
		t.Error("the client secret is not correct")
	}
}

func TestGetUser(t *testing.T) {
	if err := setup(); err != nil {
		t.Error(err)
		return
	}

    oc := New("", "", "")
	user, err := oc.getUserCredentials()
	if err != nil {
		t.Error(err)
		return
	}

	if user.Username != "appusername" {
		t.Error("the username is not correct", user.Username)
	}

	if user.Password != "apppassword" {
		t.Error("the password is not correct", user.Password)
	}
}

func TestAuthenticate(t *testing.T) {
	oas := httptest.NewServer(successHandler)
	oauthClient := New("", oas.URL, "scope0 scope1")
	authToken, err := oauthClient.Token()

	if err != nil {
		t.Error(err)
	}

	if authToken != testToken {
		t.Error("invalid token", authToken)
	}
}

func TestAuthenticateFail(t *testing.T) {
	oas := httptest.NewServer(failureHandler)
	oauthClient := New("", oas.URL, "scope0 scope1")
	authToken, err := oauthClient.Token()

	if err == nil {
		t.Error("failed to fail")
	}

	if authToken != "" {
		t.Error("invalid token", authToken)
	}
}
