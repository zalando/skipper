package logging

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
)

type loggingWriter struct {
	writer http.ResponseWriter
	code   int
	bytes  int64
}

func (lw *loggingWriter) Write(data []byte) (count int, err error) {
	count, err = lw.writer.Write(data)
	lw.bytes += int64(count)
	return
}

func (lw *loggingWriter) WriteHeader(code int) {
	lw.writer.WriteHeader(code)
	if code == 0 {
		code = 200
	}
	lw.code = code
}

func (lw *loggingWriter) Header() http.Header {
	return lw.writer.Header()
}

func (lw *loggingWriter) Flush() {
	lw.writer.(http.Flusher).Flush()
}

func (lw *loggingWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hij, ok := lw.writer.(http.Hijacker)
	if ok {
		return hij.Hijack()
	}
	return nil, nil, fmt.Errorf("could not hijack connection")
}
