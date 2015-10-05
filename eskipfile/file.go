package eskipfile

import (
	"io/ioutil"
)

type DataClient chan string

func New(path string) (DataClient, error) {
	content, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	c := make(DataClient)
	go func() { c <- string(content) }()

	return c, nil
}

func (dc DataClient) Receive() <-chan string { return dc }
