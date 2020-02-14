package diag

import (
	"bytes"
	"io/ioutil"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/filters"
)

type logHeader struct{}

func NewLogHeader() filters.Spec                                     { return logHeader{} }
func (logHeader) Name() string                                       { return "logHeader" }
func (logHeader) CreateFilter([]interface{}) (filters.Filter, error) { return logHeader{}, nil }
func (logHeader) Response(filters.FilterContext)                     {}

func (logHeader) Request(ctx filters.FilterContext) {
	req := ctx.Request()
	body := req.Body
	defer func() {
		req.Body = body
	}()

	req.Body = ioutil.NopCloser(bytes.NewBuffer(nil))
	buf := bytes.NewBuffer(nil)
	if err := req.Write(buf); err != nil {
		log.Println(err)
		return
	}

	log.Println(buf.String())
}
