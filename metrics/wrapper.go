package metrics

import "net/http"

type metricsWrapper struct {
	writer http.ResponseWriter
	code   int
	bytes  int64
}

func (mw *metricsWrapper) Write(data []byte) (count int, err error) {
	count, err = mw.writer.Write(data)
	mw.bytes = int64(count)
	return
}

func (mw *metricsWrapper) WriteHeader(code int) {
	mw.writer.WriteHeader(code)
	if code == 0 {
		code = 200
	}
	mw.code = code
}

func (mw *metricsWrapper) Header() http.Header {
	return mw.writer.Header()
}
