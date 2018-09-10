package logging

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
)

type LoggingWriter struct {
	writer http.ResponseWriter
	bytes  int64
}

func NewLoggingWriter(writer http.ResponseWriter) *LoggingWriter {
	return &LoggingWriter{writer: writer}
}

func (lw *LoggingWriter) Write(data []byte) (count int, err error) {
	count, err = lw.writer.Write(data)
	lw.bytes += int64(count)
	return
}

func (lw *LoggingWriter) WriteHeader(code int) {
	lw.writer.WriteHeader(code)
}

func (lw *LoggingWriter) Header() http.Header {
	return lw.writer.Header()
}

func (lw *LoggingWriter) Flush() {
	lw.writer.(http.Flusher).Flush()
}

func (lw *LoggingWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hij, ok := lw.writer.(http.Hijacker)
	if ok {
		return hij.Hijack()
	}
	return nil, nil, fmt.Errorf("could not hijack connection")
}

func (lw *LoggingWriter) GetBytes() int64 {
	return lw.bytes
}
