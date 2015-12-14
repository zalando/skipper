package innkeeper

import (
	"github.com/zalando/skipper/oauth"
	"testing"
)

func TestCreateInnkeeperAuthenticationFixedToken(t *testing.T) {
	options := AuthOptions{InnkeeperAuthToken: "helloToken"}
	auth := CreateInnkeeperAuthentication(options)
	token, _ := auth.GetToken()
	if token != "helloToken" {
		t.Error("wrong fixed token")
	}
}

func TestCreateInnkeeperAuthenticationClient(t *testing.T) {
	options := AuthOptions{InnkeeperAuthToken: "",
		OAuthCredentialsDir: "dir",
		OAuthUrl:            "url",
		OAuthScope:          "scope"}
	auth := CreateInnkeeperAuthentication(options)

	if _, ok := auth.(*oauth.OAuthClient); !ok {
		t.Error("wrong authentication client type")
	}
}
