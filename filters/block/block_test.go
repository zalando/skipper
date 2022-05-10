package block

import (
	"net/http"
	"strings"
	"testing"
)

func (r *nonBlockingReader) Close() error {
	return nil
}

func (r *infiniteReader) Close() error {
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
			err:     ErrBlocked,
		},
		{
			name:    "small string without match",
			content: "foxi",
			err:     nil,
		},
		{
			name:    "small string with match",
			content: "fox.class.foo.blah",
			err:     ErrBlocked,
		},
		{
			name:    "long string",
			content: strings.Repeat("A", 8192),
			err:     nil,
		}} {
		t.Run(tt.name, func(t *testing.T) {
			r := &nonBlockingReader{initialContent: []byte(tt.content)}
			bmb := newMatcher(r, []string{".class"}, defaultMaxEditorBufferSize, maxBufferBestEffort)
			t.Logf("Content: %s", r.initialContent)
			p := make([]byte, len(r.initialContent))
			n, err := bmb.Read(p)
			t.Logf("P after reading is: %s", p)
			if err != nil {
				if err == ErrBlocked {
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
		return strings.Repeat(source[:2], len/2) // partially matches target
	}

	fakematch := func(source string, len int) string {
		return strings.Repeat(source, len/2) // partially matches target
	}

	for _, tt := range []struct {
		name    string
		tomatch string
		bm      []byte
	}{
		{
			name:    "Small",
			tomatch: ".class",
			bm:      []byte(fake(".class", 10)),
		},
		{
			name:    "Small with match",
			tomatch: ".class",
			bm:      []byte(fakematch(".class", 10)),
		},
		{
			name:    "Medium",
			tomatch: ".class",
			bm:      []byte(fake(".class", 1000)),
		},
		{
			name:    "Large",
			tomatch: ".class",
			bm:      []byte(fake(".class", 10000)),
		}} {
		b.Run(tt.name, func(b *testing.B) {
			b.Logf("Request Content: %s", tt.bm)
			target := &nonBlockingReader{initialContent: tt.bm}
			r := &http.Request{
				Body: target,
			}
			bmb := newMatcher(r.Body, []string{tt.tomatch}, defaultMaxEditorBufferSize, maxBufferBestEffort)
			p := make([]byte, len(target.initialContent))
			b.Logf("Number of loops: %b", b.N)
			for n := 0; n < b.N; n++ {
				bmb.Read(p)
			}
		})
	}
}
