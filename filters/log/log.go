/*
Package log provides a request logging filter, usable also for audit logging.
*/
package log

import (
	"bytes"
	"encoding/json"
	log "github.com/sirupsen/logrus"
	"io"
	"os"

	"github.com/zalando/skipper/filters"
)

const (
	AuditLogName        = "auditLog"
	AuthUserKey         = "auth-user"
	AuthRejectReasonKey = "auth-reject-reason"
)

type auditLog struct {
	writer     io.Writer
	maxBodyLog int
}

type teeBody struct {
	body      io.ReadCloser
	buffer    *bytes.Buffer
	teeReader io.Reader
	maxTee    int
}

type auditDoc struct {
	Method      string         `json:"method"`
	Path        string         `json:"path"`
	Status      int            `json:"status"`
	AuthStatus  *authStatusDoc `json:"authStatus,omitempty"`
	RequestBody string         `json:"requestBody,omitempty"`
}

type authStatusDoc struct {
	User     string `json:"user,omitempty"`
	Rejected bool   `json:"rejected"`
	Reason   string `json:"reason,omitempty"`
}

func newTeeBody(rc io.ReadCloser, maxTee int) io.ReadCloser {
	b := bytes.NewBuffer(nil)
	tb := &teeBody{
		body:   rc,
		buffer: b,
		maxTee: maxTee}
	tb.teeReader = io.TeeReader(rc, tb)
	return tb
}

func (tb *teeBody) Read(b []byte) (int, error) { return tb.teeReader.Read(b) }
func (tb *teeBody) Close() error               { return tb.body.Close() }

func (tb *teeBody) Write(b []byte) (int, error) {
	if tb.maxTee < 0 {
		return tb.buffer.Write(b)
	}

	wl := len(b)
	if wl >= tb.maxTee {
		wl = tb.maxTee
	}

	n, err := tb.buffer.Write(b[:wl])
	if err != nil {
		return n, err
	}

	tb.maxTee -= n

	// lie to avoid short write
	return len(b), nil
}

// Creates an auditLog filter specification. It expects a writer for
// the output of the log entries.
//
//     spec := NewAuditLog()
func NewAuditLog() filters.Spec {
	return &auditLog{writer: os.Stderr}
}

func (al *auditLog) Name() string { return AuditLogName }

func (al *auditLog) CreateFilter(args []interface{}) (filters.Filter, error) {
	if len(args) == 0 {
		return al, nil
	}

	if mbl, ok := args[0].(float64); ok {
		return &auditLog{writer: al.writer, maxBodyLog: int(mbl)}, nil
	} else {
		return nil, filters.ErrInvalidFilterParameters
	}
}

func (al *auditLog) Request(ctx filters.FilterContext) {
	if al.maxBodyLog != 0 {
		ctx.Request().Body = newTeeBody(ctx.Request().Body, al.maxBodyLog)
	}
}

func (al *auditLog) Response(ctx filters.FilterContext) {
	req := ctx.Request()

	oreq := ctx.OriginalRequest()
	rsp := ctx.Response()
	doc := auditDoc{
		Method: oreq.Method,
		Path:   oreq.URL.Path,
		Status: rsp.StatusCode}

	sb := ctx.StateBag()
	au, _ := sb[AuthUserKey].(string)
	rr, _ := sb[AuthRejectReasonKey].(string)
	if au != "" || rr != "" {
		doc.AuthStatus = &authStatusDoc{User: au}
		if rr != "" {
			doc.AuthStatus.Rejected = true
			doc.AuthStatus.Reason = rr
		}
	}

	if tb, ok := req.Body.(*teeBody); ok {
		if tb.maxTee < 0 {
			io.Copy(tb.buffer, tb.body)
		} else {
			io.CopyN(tb.buffer, tb.body, int64(tb.maxTee))
		}

		if tb.buffer.Len() > 0 {
			doc.RequestBody = tb.buffer.String()
		}
	}

	enc := json.NewEncoder(al.writer)
	err := enc.Encode(&doc)
	if err != nil {
		log.Println(err)
	}
}
