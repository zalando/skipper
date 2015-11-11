package logging

import "net/http"

type loggingWrapper struct {
	writer http.ResponseWriter
	code   int
	bytes  int64
}

func (lw *loggingWrapper) Write(data []byte) (count int, err error) {
	count, err = lw.writer.Write(data)
	lw.bytes = int64(count)
	return
}

func (lw *loggingWrapper) WriteHeader(code int) {
	lw.writer.WriteHeader(code)
	if code == 0 {
		code = 200
	}
	lw.code = code
}

func (lw *loggingWrapper) Header() http.Header {
	return lw.writer.Header()
}
