package main

type endpointType string

const (
	ephttp  endpointType = "http"
	ephttps endpointType = "https"
)

type server struct {
	url string
}

type backend struct {
	typ     endpointType
	servers []server
}

type frontend struct {
	typ        endpointType
	backendId  string
	route      string
	middleware []string
}

type middleware struct {
	id       string
	priority int
	typ      string // ???
	args     interface{}
}

type settings interface {
	getBackends() []backend
	getFrontends() []frontend
	getMiddleware() []middleware
}

type settingsStruct struct {
	backends   []backend
	frontends  []frontend
	middleware []middleware
}

type etcdClient struct {
	settings chan settings
}

func (s *settingsStruct) getBackends() []backend {
	return s.backends
}

func (s *settingsStruct) getFrontends() []frontend {
	return s.frontends
}

func (s *settingsStruct) getMiddleware() []middleware {
	return s.middleware
}

func makeEtcdClient() *etcdClient {
	return &etcdClient{make(chan settings)}
}

func (ec *etcdClient) feedSettings() {
	testSettings := &settingsStruct{
		backends: []backend{
			backend{
				typ: ephttp,
				servers: []server{
					server{url: "http://localhost:9999/slow"}}}},
		frontends:  []frontend{},
		middleware: []middleware{}}

	for {
		ec.settings <- testSettings
	}
}

func (ec *etcdClient) start() {
	go ec.feedSettings()
}

func (ec *etcdClient) getSettings() <-chan settings {
	return ec.settings
}
