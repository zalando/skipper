package block

import (
	"net/http"
	"strings"
	"testing"

	"github.com/zalando/skipper/proxy"
)

type nonBlockingReader struct {
	initialContent []byte
}

func (r *nonBlockingReader) Read(p []byte) (int, error) {
	n := copy(p, r.initialContent)
	r.initialContent = r.initialContent[n:]
	return n, nil
}

func (r *nonBlockingReader) Close() error {
	return nil
}

func TestBlock(t *testing.T) {
	for _, tt := range []struct {
		name    string
		content string
		err     error
	}{
		{
			name:    "small string",
			content: ".class",
			err:     proxy.ErrBlocked,
		},
		{
			name:    "small string without match",
			content: "foxi",
			err:     nil,
		},
		{
			name:    "small string with match",
			content: "fox.class.foo.blah",
			err:     proxy.ErrBlocked,
		},
		{
			name:    "long string",
			content: strings.Repeat("A", 8192),
			err:     nil,
		}} {
		t.Run(tt.name, func(t *testing.T) {
			r := &nonBlockingReader{initialContent: []byte(tt.content)}
			toblockList := []toblockKeys{{str: []byte(".class")}}

			bmb := newMatcher(r, toblockList, 2097152, maxBufferBestEffort)

			t.Logf("Content: %s", r.initialContent)
			p := make([]byte, len(r.initialContent))
			n, err := bmb.Read(p)
			if err != nil {
				if err == proxy.ErrBlocked {
					t.Logf("Stop! Request has some blocked content!")
				} else {
					t.Errorf("Failed to read: %v", err)
				}
			} else if n != len(tt.content) {
				t.Errorf("Failed to read content length %d, got %d", len(tt.content), n)
			}

		})
	}

}

func BenchmarkBlock(b *testing.B) {

	fake := func(source string, len int) string {
		return strings.Repeat(source[:2], len) // partially matches target
	}

	fakematch := func(source string, len int) string {
		return strings.Repeat(source, len) // matches target
	}

	for _, tt := range []struct {
		name    string
		tomatch []byte
		bm      []byte
	}{
		{
			name:    "Small Stream without blocking",
			tomatch: []byte(".class"),
			bm:      []byte(fake(".class", 1<<20)), // Test with 1Mib
		},
		{
			name:    "Small Stream with blocking",
			tomatch: []byte(".class"),
			bm:      []byte(fakematch(".class", 1<<20)),
		},
		{
			name:    "Medium Stream without blocking",
			tomatch: []byte(".class"),
			bm:      []byte(fake(".class", 1<<24)), // Test with ~10Mib
		},
		{
			name:    "Medium Stream with blocking",
			tomatch: []byte(".class"),
			bm:      []byte(fakematch(".class", 1<<24)),
		},
		{
			name:    "Large Stream without blocking",
			tomatch: []byte(".class"),
			bm:      []byte(fake(".class", 1<<27)), // Test with ~100Mib
		},
		{
			name:    "Large Stream with blocking",
			tomatch: []byte(".class"),
			bm:      []byte(fakematch(".class", 1<<27)),
		}} {
		b.Run(tt.name, func(b *testing.B) {
			target := &nonBlockingReader{initialContent: tt.bm}
			r := &http.Request{
				Body: target,
			}
			toblockList := []toblockKeys{{str: tt.tomatch}}
			bmb := newMatcher(r.Body, toblockList, 2097152, maxBufferBestEffort)
			p := make([]byte, len(target.initialContent))
			b.Logf("Number of loops: %b", b.N)
			for n := 0; n < b.N; n++ {
				_, err := bmb.Read(p)
				if err != nil {
					return
				}
			}
		})
	}
}
