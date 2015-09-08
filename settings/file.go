package settings

import (
	"github.com/zalando/skipper/skipper"
	"io/ioutil"
)

type fileDataClient struct {
	channel <-chan skipper.RawData
}

type rawString struct {
	data string
}

func (r rawString) Get() string {
	return r.data
}

func (f *fileDataClient) Receive() <-chan skipper.RawData {
	return f.channel
}

func MakeFileDataClient(path string) (skipper.DataClient, error) {
	content, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	c := make(chan skipper.RawData)
	go func() {
		c <- rawString{string(content)}
	}()
	return &fileDataClient{c}, nil
}
