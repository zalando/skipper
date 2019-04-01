/*
Package log provides a request logging filter, usable also for
audit logging. Audit logging is showing who did a request in case of
OAuth2 provider returns a "uid" key and value.
*/
package log

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"os"
	"regexp"
	"strings"

	log "github.com/sirupsen/logrus"

	"github.com/zalando/skipper/filters"
)

const (
	// AuditLogName is the filter name seen by the user
	AuditLogName = "auditLog"
	// AuthUserKey is used by the auth package to set the user
	// information into the state bag to pass the information to
	// the auditLog filter.
	AuthUserKey = "auth-user"
	// AuthRejectReasonKey is used by the auth package to set the
	// reject reason information into the state bag to pass the
	// information to the auditLog filter.
	AuthRejectReasonKey = "auth-reject-reason"
	// UnverifiedAuditLogName is the filtername seen by the user
	UnverifiedAuditLogName = "unverifiedAuditLog"

	// UnverifiedAuditHeader is the name of the header added to the request which contains the unverified audit details
	UnverifiedAuditHeader        = "X-Unverified-Audit"
	authHeaderName               = "Authorization"
	authHeaderPrefix             = "Bearer "
	defaultSub                   = "<invalid-sub>"
	defaultUnverifiedAuditLogKey = "sub"
)

var (
	re = regexp.MustCompile("^[a-zA-z0-9_/:?=&%@.#-]*$")
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

// NewAuditLog creates an auditLog filter specification. It expects a
// maxAuditBody attribute to limit the size of the log. It will use
// os.Stderr as writer for the output of the log entries.
//
//     spec := NewAuditLog(1024)
func NewAuditLog(maxAuditBody int) filters.Spec {
	return &auditLog{
		writer:     os.Stderr,
		maxBodyLog: maxAuditBody,
	}
}

func (al *auditLog) Name() string { return AuditLogName }

// CreateFilter has no arguments. It creates the filter if the user
// specifies auditLog() in their route.
func (al *auditLog) CreateFilter(args []interface{}) (filters.Filter, error) {
	if len(args) != 0 {
		return nil, filters.ErrInvalidFilterParameters
	}

	return &auditLog{writer: al.writer, maxBodyLog: al.maxBodyLog}, nil
}

func (al *auditLog) Request(ctx filters.FilterContext) {
	if al.maxBodyLog != 0 {
		ctx.Request().Body = newTeeBody(ctx.Request().Body, al.maxBodyLog)
	}
}

func (al *auditLog) Response(ctx filters.FilterContext) {
	req := ctx.Request()
	rsp := ctx.Response()
	doc := auditDoc{
		Method: req.Method,
		Path:   req.URL.Path,
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
		log.Errorf("Failed to json encode auditDoc: %v", err)
	}
}

type (
	unverifiedAuditLogSpec struct {
	}
	unverifiedAuditLogFilter struct {
		TokenKey string
	}
)

// NewUnverifiedAuditLog logs "Sub" of the middle part of a JWT Token. Or else, logs the requested JSON key if present
func NewUnverifiedAuditLog() filters.Spec { return &unverifiedAuditLogSpec{} }

func (ual *unverifiedAuditLogSpec) Name() string { return UnverifiedAuditLogName }

// CreateFilter has no arguments. It creates the filter if the user
// specifies unverifiedAuditLog() in their route.
func (ual *unverifiedAuditLogSpec) CreateFilter(args []interface{}) (filters.Filter, error) {
	var len = len(args)
	if len == 0 {
		return &unverifiedAuditLogFilter{TokenKey: defaultUnverifiedAuditLogKey}, nil
	} else if len == 1 {
		keyName, ok := args[0].(string)
		if !ok {
			return nil, filters.ErrInvalidFilterParameters
		}

		return &unverifiedAuditLogFilter{TokenKey: keyName}, nil
	}

	return nil, filters.ErrInvalidFilterParameters
}

func (ual *unverifiedAuditLogFilter) Request(ctx filters.FilterContext) {
	req := ctx.Request()
	ahead := req.Header.Get(authHeaderName)
	if !strings.HasPrefix(ahead, authHeaderPrefix) {
		return
	}

	fields := strings.FieldsFunc(ahead, func(r rune) bool {
		return r == []rune(".")[0]
	})
	if len(fields) == 3 {
		sDec, err := base64.RawURLEncoding.DecodeString(fields[1])
		if err != nil {
			return
		}

		var j map[string]interface{}
		err = json.Unmarshal(sDec, &j)
		if err != nil {
			return
		}

		if k, ok := j[ual.TokenKey]; ok {
			if v, ok2 := k.(string); ok2 {
				req.Header.Add(UnverifiedAuditHeader, cleanSub(v))
			}
		}
	}
}

func (*unverifiedAuditLogFilter) Response(filters.FilterContext) {}

func cleanSub(s string) string {
	if re.MatchString(s) {
		return s
	}
	return defaultSub
}
