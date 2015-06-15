package settings

import "testing"
import "skipper/skipper"

type mockRawData struct {
	data map[string]interface{}
}

type mockDataClient struct {
	data *mockRawData
	get  chan skipper.RawData
}

func (rd *mockRawData) GetTestData() map[string]interface{} {
	return rd.data
}

func makeMockDataClient() *mockDataClient {
	return &mockDataClient{
		get: make(chan skipper.RawData)}
}

func (dc *mockDataClient) setData(d map[string]interface{}) {
	go func() {
		dc.get <- &mockRawData{d}
	}()
}

func (dc *mockDataClient) Get() <-chan skipper.RawData {
	return dc.get
}

func TestGetUpdatedSettings(t *testing.T) {
	// create mock data client
	dc := makeMockDataClient()
	dc.setData(map[string]interface{}{
		"backends": map[string]interface{}{"test-0": "test-host-0"},
		"frontends": []interface{}{
			map[string]interface{}{
				"route":     "Path(`/test`)",
				"backendId": "test-0"}}})
	// set initial mock data
	// create settings source
	// receive initial settings
	// update data in data client
	// receive updated settings
}

func TestWaitForInitialSettings(t *testing.T) {
	// create mock data client
	// create settings source
	// wait for initial settings
	// set initial settings
	// receive initial settings
}
