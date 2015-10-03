package testdataclient

type TestDataClient struct {
	data chan string
}

func New(data string) *TestDataClient {
	dc := &TestDataClient{make(chan string)}
	dc.Feed(data)
	return dc
}

func (dc *TestDataClient) Receive() <-chan string { return dc.data }
func (dc *TestDataClient) Feed(data string)       { go func() { dc.data <- data }() }
