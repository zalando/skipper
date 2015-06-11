package settingssource

import "skipper/settings"
import "github.com/mailgun/route"
import "net/http"

const defaultFeedBufferSize = 32

type backend struct {
	url string
}

type settingst struct {
	routes route.Router
}

type Source struct {
	dataClient settings.DataClient
	current    settings.Settings
	get        chan settings.Settings
}

func getBufferSize() int {
	// todo: return defaultFeedBufferSize when not dev env
	return 0
}

func processRaw(rd settings.RawData) settings.Settings {
	s := &settingst{routes: route.New()}

	mapping := rd.GetTestMapping()
	for m, u := range mapping {
		s.routes.AddRoute(m, &backend{u})
	}

	return s
}

func MakeSource(dc settings.DataClient) *Source {
	s := &Source{
		dataClient: dc,
		get:        make(chan settings.Settings, getBufferSize())}
	go s.feed()
	return s
}

func (ss *Source) Get() <-chan settings.Settings {
	return ss.get
}

func (ss *Source) feed() {

	// initial settings
	rd := <-ss.dataClient.Get()
	ss.current = processRaw(rd)

	for {
		select {
		case rd = <-ss.dataClient.Get():
			ss.current = processRaw(rd)
		case ss.get <- ss.current:
		}
	}
}

func (s *settingst) Route(r *http.Request) (settings.Backend, error) {
	b, err := s.routes.Route(r)
	if b == nil || err != nil {
		return nil, err
	}

	return b.(settings.Backend), nil
}

func (b *backend) Url() string {
	return b.url
}
