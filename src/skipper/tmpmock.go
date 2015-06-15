package main

import "skipper/skipper"

type testData struct {
	data map[string]interface{}
}

type mockDataClient struct {
	receive chan skipper.RawData
}

type mockMiddlewareRegistry struct {
}

func (rd *testData) GetTestData() map[string]interface{} {
	return map[string]interface{}{
		"backends": map[string]interface{}{"hello": "http://localhost:9999/slow"},
		"frontends": []interface{}{
			map[string]interface{}{
				"route":     "Path(\"/hello\")",
				"backendId": "hello"}}}
}

func makeMockDataClient() *mockDataClient {
	sc := &mockDataClient{make(chan skipper.RawData)}
	go func() {
		sc.receive <- &testData{}
	}()

	return sc
}

func (sc *mockDataClient) Receive() <-chan skipper.RawData {
	return sc.receive
}

func (mwr *mockMiddlewareRegistry) Add(mw ...skipper.Middleware) {
}

func (mwr *mockMiddlewareRegistry) Get(name string) skipper.Middleware {
	return nil
}

func (mwr *mockMiddlewareRegistry) Remove(name string) {
}
