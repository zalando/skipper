package net

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSizeOfRequestHeaderIsGoodEnough(t *testing.T) {
	t.Run("minimal", func(t *testing.T) {
		r, _ := http.NewRequest("GET", "http://example.com/", nil)
		r.Header.Set("User-Agent", "test-agent")

		as := SizeOfRequestHeader(r)
		es := exactSizeOfRequestHeader(r)

		t.Logf("Size: %d, exact size: %d", as, es)

		assert.Equal(t, as, es)
	})

	t.Run("multivalue", func(t *testing.T) {
		r, _ := http.NewRequest("GET", "http://example.com/", nil)
		r.Header.Set("User-Agent", "test-agent")
		r.Header.Add("Foo", "Value1")
		r.Header.Add("Foo", "Value2")

		as := SizeOfRequestHeader(r)
		es := exactSizeOfRequestHeader(r)

		r = r.Clone(context.Background())
		r.Body = nil // discard body
		var b bytes.Buffer
		r.Write(&b)
		t.Logf("b: %s", b.String())

		t.Logf("Size: %d, exact size: %d", as, es)

		assert.Equal(t, as, es)
	})

	t.Run("browser", func(t *testing.T) {
		r := browserRequest()

		as := SizeOfRequestHeader(r)
		es := exactSizeOfRequestHeader(r)

		t.Logf("Size: %d, exact size: %d", as, es)

		assert.Equal(t, as, es)
	})
}

func BenchmarkSizeOfRequestHeader(b *testing.B) {
	b.Run("exact", func(b *testing.B) {
		r := browserRequest()

		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			exactSizeOfRequestHeader(r)
		}
	})

	b.Run("fast", func(b *testing.B) {
		r := browserRequest()

		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			SizeOfRequestHeader(r)
		}
	})
}

// browserRequest returns request copied as cURL and masked from a real browser request.
func browserRequest() *http.Request {
	r, _ := http.NewRequest("GET", "https://www.google.com/search?q=what+is+the+maximum+length+of+query+string+in+url&foo="+strings.Repeat("X", 623), nil)
	r.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7")
	r.Header.Set("Accept-Language", "en-US,en;q=0.9")
	r.Header.Set("Available-Dictionary", strings.Repeat("X", 50))
	r.Header.Set("Cache-Control", "max-age=0")
	r.Header.Set("Cookie", "__gsas="+strings.Repeat("X", 2400))
	r.Header.Set("Downlink", "10")
	r.Header.Set("Priority", "u=0, i")
	r.Header.Set("Referer", "https://www.google.com/")
	r.Header.Set("Rtt", "50")
	r.Header.Set("Sec-Ch-Prefers-Color-Scheme", "light")
	r.Header.Set("Sec-Ch-Ua", `"Google Chrome";v="135", "Not-A.Brand";v="8", "Chromium";v="135"`)
	r.Header.Set("Sec-Ch-Ua-Arch", `"x86"`)
	r.Header.Set("Sec-Ch-Ua-Bitness", `"64"`)
	r.Header.Set("Sec-Ch-Ua-Form-Factors", `"Desktop"`)
	r.Header.Set("Sec-Ch-Ua-Full-Version", `"135.0.7049.114"`)
	r.Header.Set("Sec-Ch-Ua-Full-Version-List", `"Google Chrome";v="135.0.7049.114", "Not-A.Brand";v="8.0.0.0", "Chromium";v="135.0.7049.114"`)
	r.Header.Set("Sec-Ch-Ua-Mobile", "?0")
	r.Header.Set("Sec-Ch-Ua-Model", `""`)
	r.Header.Set("Sec-Ch-Ua-Platform", `"Linux"`)
	r.Header.Set("Sec-Ch-Ua-Platform-Version", `"6.11.0"`)
	r.Header.Set("Sec-Ch-Ua-Wow64", "?0")
	r.Header.Set("Sec-Fetch-Dest", "document")
	r.Header.Set("Sec-Fetch-Mode", "navigate")
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	r.Header.Set("Sec-Fetch-User", "?1")
	r.Header.Set("Upgrade-Insecure-Requests", "1")
	r.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/135.0.0.0 Safari/537.36")
	r.Header.Set("X-Browser-Channel", "stable")
	r.Header.Set("X-Browser-Copyright", "Copyright 2025 Google LLC. All rights reserved.")
	r.Header.Set("X-Browser-Validation", strings.Repeat("X", 30))
	r.Header.Set("X-Browser-Year", "2025")
	r.Header.Set("X-Client-Data", strings.Repeat("X", 80))
	return r
}
