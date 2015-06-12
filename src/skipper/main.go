package main

import "log"
import "net/http"
import "skipper/proxy"
import "skipper/settings"
import "skipper/skipper"

type RawData struct {
	mapping map[string]string
}

type Mock struct {
	RawData *RawData
	get     chan skipper.RawData
}

func TempMock() *Mock {
	m := &Mock{
		&RawData{
			map[string]string{
				"Path(\"/hello<v>\")": "http://localhost:9999/slow"}},
		make(chan skipper.RawData)}
	go m.feed()
	return m
}

func (m *Mock) feed() {
	nc := make(chan int)
	for {
		m.get <- m.RawData
		<-nc
	}
}

func (m *Mock) Get() <-chan skipper.RawData {
	return m.get
}

func (rd *RawData) GetTestData() map[string]interface{} {
	return map[string]interface{}{
		"backends": map[string]interface{}{"hello": "http://localhost:9999/slow"},
		"frontends": []interface{}{
			map[string]interface{}{
				"route":     "Path(\"/hello\")",
				"backendId": "hello"}}}
}

func main() {
	e := TempMock()
	ss := settings.MakeSource(e, nil)
	p := proxy.Make(ss)
	s := <-ss.Get()
	log.Fatal(http.ListenAndServe(s.Address(), p))
}
