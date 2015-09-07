package settings

import (
	"io/ioutil"
)

type FileDataClient struct {
	channel <-chan string
}

func (f *FileDataClient) Receive() <-chan string {
	return f.channel
}

func MakeFileDataClient(path string) (*FileDataClient, error) {
	content, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	c := make(chan string)
	go func() {
		c <- string(content)
	}()
	return &FileDataClient{c}, nil
}
