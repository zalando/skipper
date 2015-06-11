package settings

import "net/http"

type RawData interface {
	GetTestMapping() map[string]string
}

type DataClient interface {
	Get() <-chan RawData
}

type Backend interface {
	Url() string
}

type Settings interface {
	Route(*http.Request) (Backend, error)
}

type Source interface {
	Get() <-chan Settings
}
