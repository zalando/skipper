/*
Package log provides a request logging filter, usable also for
audit logging. Audit logging is showing who did a request in case of
OAuth2 provider returns a "uid" key and value.
*/
package log

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/jwt"
)

const (
	// Deprecated, use filters.AuditLogName instead
	AuditLogName = filters.AuditLogName

	// AuthUserKey is used by the auth package to set the user
	// information into the state bag to pass the information to
	// the auditLog filter.
	AuthUserKey = "auth-user"
	// AuthRejectReasonKey is used by the auth package to set the
	// reject reason information into the state bag to pass the
	// information to the auditLog filter.
	AuthRejectReasonKey = "auth-reject-reason"

	// Deprecated, use filters.UnverifiedAuditLogName instead
	UnverifiedAuditLogName = filters.UnverifiedAuditLogName

	// UnverifiedAuditHeader is the name of the header added to the request which contains the unverified audit details
	UnverifiedAuditHeader        = "X-Unverified-Audit"
	authHeaderName               = "Authorization"
	authHeaderPrefix             = "Bearer "
	defaultSub                   = "<invalid-sub>"
	defaultUnverifiedAuditLogKey = "sub"
)

var (
	re = regexp.MustCompile("^[a-zA-Z0-9_/:?=&%@.#-]*$")
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

	wl := min(len(b), tb.maxTee)

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
//	spec := NewAuditLog(1024)
func NewAuditLog(maxAuditBody int) filters.Spec {
	return &auditLog{
		writer:     os.Stderr,
		maxBodyLog: maxAuditBody,
	}
}

func (al *auditLog) Name() string { return filters.AuditLogName }

// CreateFilter has no arguments. It creates the filter if the user
// specifies auditLog() in their route.
func (al *auditLog) CreateFilter(args []any) (filters.Filter, error) {
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
		ctx.Logger().Errorf("Failed to json encode auditDoc: %v", err)
	}
}

type (
	unverifiedAuditLogSpec   struct{}
	unverifiedAuditLogFilter struct {
		TokenKeys []string
	}
)

// NewUnverifiedAuditLog logs "Sub" of the middle part of a JWT Token. Or else, logs the requested JSON key if present
func NewUnverifiedAuditLog() filters.Spec { return &unverifiedAuditLogSpec{} }

func (ual *unverifiedAuditLogSpec) Name() string { return filters.UnverifiedAuditLogName }

// CreateFilter has no arguments. It creates the filter if the user
// specifies unverifiedAuditLog() in their route.
func (ual *unverifiedAuditLogSpec) CreateFilter(args []any) (filters.Filter, error) {
	var len = len(args)
	if len == 0 {
		return &unverifiedAuditLogFilter{TokenKeys: []string{defaultUnverifiedAuditLogKey}}, nil
	}

	keys := make([]string, len)
	for i := range len {
		keyName, ok := args[i].(string)
		if !ok {
			return nil, filters.ErrInvalidFilterParameters
		}
		keys[i] = keyName
	}

	return &unverifiedAuditLogFilter{TokenKeys: keys}, nil
}

func (ual *unverifiedAuditLogFilter) Request(ctx filters.FilterContext) {
	req := ctx.Request()
	ahead := req.Header.Get(authHeaderName)
	tv := strings.TrimPrefix(ahead, authHeaderPrefix)
	if tv == ahead {
		return
	}

	token, err := jwt.Parse(tv)
	if err != nil {
		return
	}

	for i := 0; i < len(ual.TokenKeys); i++ {
		if k, ok := token.Claims[ual.TokenKeys[i]]; ok {
			if v, ok2 := k.(string); ok2 {
				req.Header.Add(UnverifiedAuditHeader, cleanSub(v))
				return
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
