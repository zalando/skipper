package oauth

import (
	"os"
	"testing"
    "net/http"
    "net/http/httptest"
    "encoding/json"
)

const (
    clientJson = `{"client_id": "theclientid", "client_secret": "clientsecret"}`
    userJson = `{"application_username": "appusername", "application_password": "apppassword"}`
    testToken = "test token"
)

func setup() error {
	os.Setenv(credentialsDir, ".")

	err := createFileWithContent("client.json", clientJson)
    if err == nil {
        err = createFileWithContent("user.json", userJson)
    }

    return err
}

var successHandler http.Handler = http.HandlerFunc(func (w http.ResponseWriter, r *http.Request) {
    failed := false
    check := func(cond bool) {
        if failed {
            return
        }

        w.WriteHeader(404)
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
	checkForm("scope", "uid fashion_store_route.read")

    w.WriteHeader(200)
    enc := json.NewEncoder(w)

    // ignore error
    enc.Encode(&authResponse{AccessToken: testToken})
})

var failureHandler http.Handler = http.HandlerFunc(func (w http.ResponseWriter, r *http.Request) {
    w.WriteHeader(404)
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
	postBody := getAuthPostBody(us)
	if postBody != "grant_type=password&password=pass&scope=uid+fashion_store_route.read&username=user" {
		t.Error("the post body is not correct", postBody)
	}
}

func TestGetCredentialsDir(t *testing.T) {
	if err := setup(); err != nil {
        t.Error(err)
        return
    }

	dir := getCredentialsDir()
	if dir == "" {
		t.Error("the dir should not be nil")
	}
}

func TestGetCredentialsJson(t *testing.T) {
	if err := setup(); err != nil {
        t.Error(err)
        return
    }

	json, err := getCredentialsData("client.json")
    if err != nil {
        t.Error(err)
        return
    }

	if string(json) != clientJson {
		t.Error("the json is not correct", json)
	}
}

func TestGetClient(t *testing.T) {
	if err := setup(); err != nil {
        t.Error(err)
        return
    }

	client, _ := getClientCredentials()
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

	user, err := getUserCredentials()
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
	oauthClient := Make(oas.URL)
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
	oauthClient := Make(oas.URL)
	authToken, err := oauthClient.Token()

    if err == nil {
        t.Error("failed to fail")
    }

    if authToken != "" {
        t.Error("invalid token", authToken)
    }
}
