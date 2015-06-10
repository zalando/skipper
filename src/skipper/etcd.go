package main

type settings interface {
    url() string
}

type settingsStruct struct {
    u string
}

type etcdClient struct {
    settings chan settings
}

func (s *settingsStruct) url() string {
    return s.u
}

func makeEtcdClient() *etcdClient {
    return &etcdClient{make(chan settings)}
}

func (ec *etcdClient) feedSettings() {
    s := &settingsStruct{"http://localhost:9999/slow"}
    for {
        ec.settings <- s
    }
}

func (ec *etcdClient) start() {
    go ec.feedSettings()
}
