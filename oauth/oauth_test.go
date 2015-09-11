package oauth

import (
	"crypto/tls"
	"log"
	"net/http"
	"os"
	"testing"
)

const clientJson = "{\"client_id\":\"theclientid\", \"client_secret\":\"clientsecret\"}"
const userJson = "{\"application_username\":\"appusername\", \"application_password\":\"apppassword\"}"

func setup() {
	os.Setenv(credentialsDir, ".")
	createFileWithContent("client.json", clientJson)
	createFileWithContent("user.json", userJson)
}

func createFileWithContent(fileName string, content string) error {
	file, err := os.Open(fileName)
	if err != nil {
		f, _ := os.Create(fileName)
		defer f.Close()
		_, err := f.WriteString(content)
		if err != nil {
			return err
		}
	} else {
		file.Close()
	}
	return nil
}

func TestGetAuthPostBody(t *testing.T) {
	us := &user{"user", "pass"}
	postBody := getAuthPostBody(us)
	log.Println(postBody)
	if postBody != "grant_type=password&password=pass&scope=uid+fashion_store_routes.read_all&username=user" {
		t.Error("the post body is not correct")
	}
}

func TestGetCredentialsDir(t *testing.T) {
	setup()
	dir := getCredentialsDir()
	log.Println(dir)
	if dir == "" {
		t.Error("the dir should not be nil")
	}
}

func TestGetCredentialsJson(t *testing.T) {
	setup()
	json, _ := getCredentialsJson("client")
	log.Println(string(json))
	if string(json) != clientJson {
		t.Error("the json is not correct")
	}
}

func TestGetClient(t *testing.T) {
	setup()
	client, _ := getClient()
	log.Println(client)
	if client.Id != "theclientid" {
		t.Error("the client id is not correct")
	}
	if client.Secret != "clientsecret" {
		t.Error("the client secret is not correct")
	}
}

func TestGetUser(t *testing.T) {
	setup()
	user, _ := getUser()
	log.Println(user)
	if user.Username != "appusername" {
		t.Error("the username is not correct")
	}
	if user.Password != "apppassword" {
		t.Error("the password is not correct")
	}
}

func TestAuthenticate(t *testing.T) {
	tr := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
	httpClient := &http.Client{Transport: tr}

	oauthClient := Make(httpClient)

	authToken, err := oauthClient.Authenticate()
	log.Printf("the token is: %s the error %s", authToken, err)

}
