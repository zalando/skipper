package diag_test

import (
	"io"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/proxy/proxytest"
	"github.com/zalando/skipper/routing"
)

func TestRepeat(t *testing.T) {
	for _, ti := range []struct {
		def      string
		expected []byte
	}{{
		def:      `repeatContent("zero length", 0)`,
		expected: []byte(""),
	}, {
		def:      `repeatContent("1", 1)`,
		expected: []byte("1"),
	}, {
		def:      `repeatContent("0123456789", 3)`,
		expected: []byte("012"),
	}, {
		def:      `repeatContent("0123456789", 30)`,
		expected: []byte("012345678901234567890123456789"),
	}, {
		def:      `repeatContentHex("0102", 5)`,
		expected: []byte{0x01, 0x02, 0x01, 0x02, 0x01},
	}, {
		def:      `repeatContentHex("68657861646563696d616c", 11)`,
		expected: []byte("hexadecimal"),
	}} {
		p := proxytest.Config{
			RoutingOptions: routing.Options{
				FilterRegistry: builtin.MakeRegistry(),
			},
			Routes: eskip.MustParse(`* -> ` + ti.def + `-> <shunt>`),
		}.Create()
		defer p.Close()

		client := p.Client()

		rsp, err := client.Get(p.URL)
		require.NoError(t, err)
		defer rsp.Body.Close()

		body, err := io.ReadAll(rsp.Body)
		require.NoError(t, err)

		assert.Equal(t, ti.expected, body)
		assert.Equal(t, strconv.Itoa(len(ti.expected)), rsp.Header.Get("Content-Length"))
	}
}

func TestRepeatInvalidArgs(t *testing.T) {
	registry := builtin.MakeRegistry()

	for _, def := range []string{
		`repeatContent()`,
		`repeatContent(1)`,
		`repeatContent("too few arguments")`,
		`repeatContent("too many arguments", 10, "extra")`,
		`repeatContent("length is not a number", "10")`,
		`repeatContent("", 10)`,
		`repeatContentHex()`,
		`repeatContentHex(1)`,
		`repeatContentHex("012", 10)`, // odd length
		`repeatContentHex("nothex", 10)`,
	} {
		t.Run(def, func(t *testing.T) {
			ff, err := eskip.ParseFilters(def)
			require.NoError(t, err)
			require.Len(t, ff, 1)

			f := ff[0]

			spec := registry[f.Name]
			_, err = spec.CreateFilter(f.Args)

			assert.Error(t, err)
		})
	}
}
