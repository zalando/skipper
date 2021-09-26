package skipper

import (
	"crypto/tls"
	"io"
	"net"
	"net/http"
	neturl "net/url"
	"os"
	"syscall"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	"golang.org/x/net/http2"

	"github.com/zalando/skipper/dataclients/routestring"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/proxy"
	"github.com/zalando/skipper/ratelimit"
	"github.com/zalando/skipper/routing"

	"github.com/stretchr/testify/require"
)

const (
	listenDelay   = 15 * time.Millisecond
	listenTimeout = 9 * listenDelay
)

type protocol int

const (
	HTTP protocol = iota
	HTTPS
	H2C
)

func (p protocol) scheme() string {
	return [...]string{"http", "https", "http"}[p]
}

func (p protocol) newClient() *http.Client {
	switch p {
	case HTTP:
		return http.DefaultClient
	case HTTPS:
		return &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
				},
			},
		}
	case H2C:
		return &http.Client{
			Transport: &http2.Transport{
				// allow http scheme
				AllowHTTP: true,
				// ignore tls.Config and dial unencrypted TCP
				DialTLS: func(network, addr string, cfg *tls.Config) (net.Conn, error) {
					return net.Dial(network, addr)
				},
			},
		}
	}
	return nil
}

func testListener() bool {
	for _, a := range os.Args {
		if a == "listener" {
			return true
		}
	}

	return false
}

func waitConn(req func() (*http.Response, error)) (*http.Response, error) {
	to := time.After(listenTimeout)
	for {
		rsp, err := req()
		if err == nil {
			return rsp, nil
		}

		select {
		case <-to:
			return nil, err
		default:
			time.Sleep(listenDelay)
		}
	}
}

func waitConnGet(proto protocol, address string) (*http.Response, error) {
	u, err := neturl.Parse("scheme://" + address)
	if err != nil {
		return nil, err
	}
	u.Scheme = proto.scheme()
	client := proto.newClient()

	return waitConn(func() (*http.Response, error) {
		return client.Get(u.String())
	})
}

func findAddress() (string, error) {
	l, err := net.ListenTCP("tcp6", &net.TCPAddr{})
	if err != nil {
		return "", err
	}

	defer l.Close()
	return l.Addr().String(), nil
}

func TestOptionsTLSConfig(t *testing.T) {
	cert, err := tls.LoadX509KeyPair("fixtures/test.crt", "fixtures/test.key")
	require.NoError(t, err)

	cert2, err := tls.LoadX509KeyPair("fixtures/test2.crt", "fixtures/test2.key")
	require.NoError(t, err)

	// empty
	o := &Options{}
	c, err := o.tlsConfig()
	require.NoError(t, err)
	require.Nil(t, c)

	// proxy tls config
	o = &Options{ProxyTLS: &tls.Config{}}
	c, err = o.tlsConfig()
	require.NoError(t, err)
	require.Equal(t, &tls.Config{}, c)

	// proxy tls config priority
	o = &Options{ProxyTLS: &tls.Config{}, CertPathTLS: "fixtures/test.crt", KeyPathTLS: "fixtures/test.key"}
	c, err = o.tlsConfig()
	require.NoError(t, err)
	require.Equal(t, &tls.Config{}, c)

	// cert key path
	o = &Options{TLSMinVersion: tls.VersionTLS12, CertPathTLS: "fixtures/test.crt", KeyPathTLS: "fixtures/test.key"}
	c, err = o.tlsConfig()
	require.NoError(t, err)
	require.Equal(t, uint16(tls.VersionTLS12), c.MinVersion)
	require.Equal(t, []tls.Certificate{cert}, c.Certificates)

	// multiple cert key paths
	o = &Options{TLSMinVersion: tls.VersionTLS13, CertPathTLS: "fixtures/test.crt,fixtures/test2.crt", KeyPathTLS: "fixtures/test.key,fixtures/test2.key"}
	c, err = o.tlsConfig()
	require.NoError(t, err)
	require.Equal(t, uint16(tls.VersionTLS13), c.MinVersion)
	require.Equal(t, []tls.Certificate{cert, cert2}, c.Certificates)
}

func TestOptionsTLSConfigInvalid(t *testing.T) {
	for _, tt := range []struct {
		name    string
		options *Options
	}{
		{"missing cert path", &Options{KeyPathTLS: "fixtures/test.key"}},
		{"missing key path", &Options{CertPathTLS: "fixtures/test.crt"}},
		{"wrong cert path", &Options{CertPathTLS: "fixtures/notFound.crt", KeyPathTLS: "fixtures/test.key"}},
		{"wrong key path", &Options{CertPathTLS: "fixtures/test.crt", KeyPathTLS: "fixtures/notFound.key"}},
		{"cert key mismatch", &Options{CertPathTLS: "fixtures/test.crt", KeyPathTLS: "fixtures/test2.key"}},
		{"multiple cert key count mismatch", &Options{CertPathTLS: "fixtures/test.crt,fixtures/test2.crt", KeyPathTLS: "fixtures/test.key"}},
		{"multiple cert key mismatch", &Options{CertPathTLS: "fixtures/test.crt,fixtures/test2.crt", KeyPathTLS: "fixtures/test2.key,fixtures/test.key"}},
		{"h2c conflicts with tls", &Options{EnableH2CPriorKnowledge: true, CertPathTLS: "fixtures/test.crt", KeyPathTLS: "fixtures/test.key"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.options.tlsConfig()
			t.Logf("tlsConfig error: %v", err)
			require.Error(t, err)
		})
	}
}

// to run this test, set `-args listener` for the test command
func TestHTTPSServer(t *testing.T) {
	// TODO: figure why sometimes cannot connect
	if !testListener() {
		t.Skip()
	}

	a, err := findAddress()
	if err != nil {
		t.Fatal(err)
	}

	o := Options{
		Address:     a,
		CertPathTLS: "fixtures/test.crt",
		KeyPathTLS:  "fixtures/test.key",
	}

	rt := routing.New(routing.Options{
		FilterRegistry: builtin.MakeRegistry(),
		DataClients:    []routing.DataClient{}})
	defer rt.Close()

	proxy := proxy.New(rt, proxy.OptionsNone)
	defer proxy.Close()
	go listenAndServe(proxy, &o)

	r, err := waitConnGet(HTTPS, o.Address)
	if r != nil {
		defer r.Body.Close()
	}
	if err != nil {
		t.Fatalf("Cannot connect to the local server for testing: %s ", err.Error())
	}
	if r.StatusCode != 404 {
		t.Fatalf("Status code should be 404, instead got: %d\n", r.StatusCode)
	}
	_, err = io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("Failed to stream response body: %v", err)
	}
}

// to run this test, set `-args listener` for the test command
func TestHTTPServer(t *testing.T) {
	// TODO: figure why sometimes cannot connect
	if !testListener() {
		t.Skip()
	}

	a, err := findAddress()
	if err != nil {
		t.Fatal(err)
	}

	o := Options{Address: a}

	rt := routing.New(routing.Options{
		FilterRegistry: builtin.MakeRegistry(),
		DataClients:    []routing.DataClient{}})
	defer rt.Close()

	proxy := proxy.New(rt, proxy.OptionsNone)
	defer proxy.Close()
	go listenAndServe(proxy, &o)
	r, err := waitConnGet(HTTP, o.Address)
	if r != nil {
		defer r.Body.Close()
	}
	if err != nil {
		t.Fatalf("Cannot connect to the local server for testing: %s ", err.Error())
	}
	if r.StatusCode != 404 {
		t.Fatalf("Status code should be 404, instead got: %d\n", r.StatusCode)
	}
	_, err = io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("Failed to stream response body: %v", err)
	}
}

func TestServerShutdownHTTP(t *testing.T) {
	o := &Options{}
	testServerShutdown(t, o, HTTP)
}

func TestServerShutdownHTTPS(t *testing.T) {
	o := &Options{
		CertPathTLS: "fixtures/test.crt",
		KeyPathTLS:  "fixtures/test.key",
	}
	testServerShutdown(t, o, HTTPS)
}

func TestServerShutdownH2C(t *testing.T) {
	o := &Options{
		EnableH2CPriorKnowledge: true,
	}
	testServerShutdown(t, o, H2C)
}

func testServerShutdown(t *testing.T, o *Options, proto protocol) {
	const shutdownDelay = 1 * time.Second

	address, err := findAddress()
	require.NoError(t, err)

	o.Address, o.WaitForHealthcheckInterval = address, shutdownDelay

	// simulate a backend that got a request and should be handled correctly
	dc, err := routestring.New(`r0: * -> latency("3s") -> inlineContent("OK") -> status(200) -> <shunt>`)
	require.NoError(t, err)

	rt := routing.New(routing.Options{
		FilterRegistry: builtin.MakeRegistry(),
		DataClients:    []routing.DataClient{dc},
	})
	defer rt.Close()

	proxy := proxy.New(rt, proxy.OptionsNone)
	defer proxy.Close()

	sigs := make(chan os.Signal, 1)
	go func() {
		err := listenAndServeQuit(proxy, o, sigs, nil, nil)
		require.NoError(t, err)
	}()

	// initiate shutdown
	sigs <- syscall.SIGTERM

	time.Sleep(shutdownDelay / 2)

	t.Logf("ongoing request passing in before shutdown")
	r, err := waitConnGet(proto, address)
	require.NoError(t, err)
	require.Equal(t, 200, r.StatusCode)

	defer r.Body.Close()

	body, err := io.ReadAll(r.Body)
	require.NoError(t, err)
	require.Equal(t, "OK", string(body))

	time.Sleep(shutdownDelay / 2)

	t.Logf("request after shutdown should fail")
	_, err = waitConnGet(proto, address)
	require.Error(t, err)
}

type (
	customRatelimitSpec   struct{ registry *ratelimit.Registry }
	customRatelimitFilter struct{}
)

func (s *customRatelimitSpec) Name() string { return "customRatelimit" }
func (s *customRatelimitSpec) CreateFilter(config []interface{}) (filters.Filter, error) {
	log.Infof("Registry: %v", s.registry)
	return &customRatelimitFilter{}, nil
}
func (f *customRatelimitFilter) Request(ctx filters.FilterContext)  {}
func (f *customRatelimitFilter) Response(ctx filters.FilterContext) {}

func Example_ratelimitRegistryBinding() {
	s := &customRatelimitSpec{}

	o := Options{
		Address:            ":9090",
		InlineRoutes:       `* -> customRatelimit() -> <shunt>`,
		EnableRatelimiters: true,
		EnableSwarm:        true,
		SwarmRedisURLs:     []string{":6379"},
		CustomFilters:      []filters.Spec{s},
		SwarmRegistry: func(registry *ratelimit.Registry) {
			s.registry = registry
		},
	}

	log.Fatal(Run(o))
	// Example functions without output comments are compiled but not executed
}
