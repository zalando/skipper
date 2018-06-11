package auth

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
)

type authClient struct {
	url *url.URL
}

func newAuthClient(baseURL string) (*authClient, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}
	return &authClient{url: u}, nil
}

func (ac *authClient) getTokenintrospect(token string) (tokenIntrospectionInfo, error) {
	info := make(tokenIntrospectionInfo)
	err := jsonPost(ac.url, token, &info)
	if err != nil {
		return nil, err
	}
	return info, err
}

func (ac *authClient) getTokeninfo(token string) (map[string]interface{}, error) {
	var a map[string]interface{}
	err := jsonGet(ac.url, token, &a)
	return a, err
}

// jsonGet requests url with access token in the URL query specified
// by accessTokenKey, if auth was given and writes into doc.
func jsonGet(url *url.URL, accessToken string, doc interface{}) error {
	req, err := http.NewRequest("GET", url.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set(authHeaderName, authHeaderPrefix+accessToken)

	rsp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}

	defer rsp.Body.Close()
	if rsp.StatusCode != 200 {
		return errInvalidToken
	}

	d := json.NewDecoder(rsp.Body)
	return d.Decode(doc)
}

// jsonPost requests url with access token in the body, if auth was given and writes into doc.
func jsonPost(u *url.URL, auth string, doc *tokenIntrospectionInfo) error {
	body := url.Values{}
	body.Add(accessTokenKey, auth)

	rsp, err := http.PostForm(u.String(), body)
	if err != nil {
		return err
	}

	defer rsp.Body.Close()
	if rsp.StatusCode != 200 {
		return errInvalidToken
	}
	buf := make([]byte, rsp.ContentLength)
	_, err = rsp.Body.Read(buf)
	if err != nil && err != io.EOF {
		return err
	}
	err = json.Unmarshal(buf, &doc)
	if err != nil {
		return err
	}
	return err
}
