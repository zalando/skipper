package kubernetes

import (
	"fmt"
	"io/ioutil"
	"time"

	"golang.org/x/oauth2"
)

type fileTokenSource struct {
	path   string
	period time.Duration
}

func (ts *fileTokenSource) Token() (*oauth2.Token, error) {
	token, err := ioutil.ReadFile(ts.path)
	if err != nil {
		return nil, fmt.Errorf("failed to read token file %q: %v", ts.path, err)
	}

	return &oauth2.Token{
		AccessToken: string(token),
		Expiry:      time.Now().Add(ts.period),
	}, nil
}
