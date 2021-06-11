// Package script provides lua scripting for skipper
package script

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"

	lua "github.com/yuin/gopher-lua"
	lua_parse "github.com/yuin/gopher-lua/parse"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/script/base64"

	"github.com/cjoudrey/gluahttp"
	"github.com/cjoudrey/gluaurl"
	gjson "layeh.com/gopher-json"
)

// InitialPoolSize is the number of lua states created initially per route
var InitialPoolSize int = 3

// MaxPoolSize is the number of lua states stored per route - there may be more parallel
// requests, but only this number is cached.
var MaxPoolSize int = 10

type luaScript struct{}

// NewLuaScript creates a new filter spec for skipper
func NewLuaScript() filters.Spec {
	return &luaScript{}
}

// Name returns the name of the filter ("lua")
func (ls *luaScript) Name() string {
	return "lua"
}

// CreateFilter creates the filter
func (ls *luaScript) CreateFilter(config []interface{}) (filters.Filter, error) {
	if len(config) == 0 {
		return nil, filters.ErrInvalidFilterParameters
	}
	src, ok := config[0].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}
	var params []string
	for _, p := range config[1:] {
		ps, ok := p.(string)
		if !ok {
			return nil, filters.ErrInvalidFilterParameters
		}
		params = append(params, ps)
	}

	s := &script{source: src, routeParams: params}
	if err := s.initScript(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *script) getState() (*lua.LState, error) {
	select {
	case L := <-s.pool:
		if L == nil {
			return nil, errors.New("pool closed")
		}
		return L, nil
	default:
		return s.newState()
	}
}

func (s *script) putState(L *lua.LState) {
	if s.pool == nil { // pool closed
		L.Close()
		return
	}
	select {
	case s.pool <- L:
	default: // pool full, close state
		L.Close()
	}
}

func (s *script) newState() (*lua.LState, error) {
	L := lua.NewState()
	L.PreloadModule("base64", base64.Loader)
	L.PreloadModule("http", gluahttp.NewHttpModule(&http.Client{}).Loader)
	L.PreloadModule("url", gluaurl.Loader)
	L.PreloadModule("json", gjson.Loader)
	L.SetGlobal("print", L.NewFunction(printToLog))
	L.SetGlobal("sleep", L.NewFunction(sleep))

	registerPropertyTypes(L)

	L.Push(L.NewFunctionFromProto(s.proto))

	err := L.PCall(0, lua.MultRet, nil)
	if err != nil {
		L.Close()
		return nil, err
	}
	return L, nil
}

func printToLog(L *lua.LState) int {
	top := L.GetTop()
	args := make([]interface{}, 0, top)
	for i := 1; i <= top; i++ {
		args = append(args, L.ToStringMeta(L.Get(i)).String())
	}
	log.Print(args...)
	return 0
}

func sleep(L *lua.LState) int {
	time.Sleep(time.Duration(L.CheckInt64(1)) * time.Millisecond)
	return 0
}

func (s *script) initScript() error {
	// Compile
	var reader io.Reader
	var name string

	if strings.HasSuffix(s.source, ".lua") {
		file, err := os.Open(s.source)
		if err != nil {
			return err
		}
		defer func() {
			if err = file.Close(); err != nil {
				log.Errorf("Failed to close lua file %s: %v", s.source, err)
			}
		}()
		reader = bufio.NewReader(file)
		name = s.source
	} else {
		reader = strings.NewReader(s.source)
		name = "<script>"
	}
	chunk, err := lua_parse.Parse(reader, name)
	if err != nil {
		return err
	}
	proto, err := lua.Compile(chunk, name)
	if err != nil {
		return err
	}
	s.proto = proto

	// Detect request and response functions
	L, err := s.newState()
	if err != nil {
		return err
	}
	defer L.Close()

	if fn := L.GetGlobal("request"); fn.Type() == lua.LTFunction {
		s.hasRequest = true
	}
	if fn := L.GetGlobal("response"); fn.Type() == lua.LTFunction {
		s.hasResponse = true
	}
	if !s.hasRequest && !s.hasResponse {
		return errors.New("at least one of `request` and `response` function must be present")
	}

	// Init state pool
	s.pool = make(chan *lua.LState, MaxPoolSize) // FIXME make configurable
	for i := 0; i < InitialPoolSize; i++ {
		L, err := s.newState()
		if err != nil {
			return err
		}
		s.putState(L)
	}
	return nil
}

type script struct {
	source                  string
	routeParams             []string
	pool                    chan *lua.LState
	proto                   *lua.FunctionProto
	hasRequest, hasResponse bool
}

func (s *script) Request(f filters.FilterContext) {
	if s.hasRequest {
		s.runFunc("request", f)
	}
}

func (s *script) Response(f filters.FilterContext) {
	if s.hasResponse {
		s.runFunc("response", f)
	}
}

func (s *script) runFunc(name string, f filters.FilterContext) {
	L, err := s.getState()
	if err != nil {
		log.Errorf("Error obtaining lua environment: %v", err)
		return
	}
	defer s.putState(L)

	pt := L.CreateTable(len(s.routeParams), len(s.routeParams))
	for i, p := range s.routeParams {
		parts := strings.SplitN(p, "=", 2)
		if len(parts) == 1 {
			parts = append(parts, "")
		}
		pt.RawSetString(parts[0], lua.LString(parts[1]))
		pt.RawSetInt(i+1, lua.LString(p))
	}

	err = L.CallByParam(
		lua.P{
			Fn:      L.GetGlobal(name),
			NRet:    0,
			Protect: true,
		},
		newProperty(L, typeContext, f),
		pt,
	)
	if err != nil {
		log.Errorf("Error calling %s from %s: %v", name, s.source, err)
	}
}

const (
	typeContext                = "filter context"
	typeContextPathParam       = typeContext + " path param"
	typeContextStateBag        = typeContext + " state bag"
	typeContextRequest         = typeContext + " request"
	typeContextRequestHeader   = typeContextRequest + " header"
	typeContextRequestCookie   = typeContextRequest + " cookie"
	typeContextRequestUrlQuery = typeContextRequest + " url_query"
	typeContextResponse        = typeContext + " response"
	typeContextResponseHeader  = typeContextResponse + " header"
)

func registerPropertyTypes(L *lua.LState) {
	registerPropertyType(L, typeContext, getContextValue, nil, nil)
	registerPropertyType(L, typeContextPathParam, getPathParam, nil, nil)
	registerPropertyType(L, typeContextStateBag, getStateBag, setStateBag, nil)

	registerPropertyType(L, typeContextRequest, getRequestValue, setRequestValue, nil)
	registerPropertyType(L, typeContextRequestHeader, getRequestHeader, setRequestHeader, iterateRequestHeader)
	registerPropertyType(L, typeContextRequestCookie, getRequestCookie, nil, iterateRequestCookie)
	registerPropertyType(L, typeContextRequestUrlQuery, getRequestURLQuery, setRequestURLQuery, iterateRequestURLQuery)

	registerPropertyType(L, typeContextResponse, getResponseValue, setResponseValue, nil)
	registerPropertyType(L, typeContextResponseHeader, getResponseHeader, setResponseHeader, iterateResponseHeader)
}

func registerPropertyType(L *lua.LState, typ string, get lua.LGFunction, set lua.LGFunction, call lua.LGFunction) {
	mt := L.NewTypeMetatable(typ)
	if get != nil {
		mt.RawSetString("__index", L.NewFunction(get))
	}
	if set != nil {
		mt.RawSetString("__newindex", L.NewFunction(set))
	}
	if call != nil {
		mt.RawSetString("__call", L.NewFunction(call))
	}
}

func newProperty(L *lua.LState, typ string, f filters.FilterContext) (u *lua.LUserData) {
	u = L.NewUserData()
	u.Value = f
	L.SetMetatable(u, L.GetTypeMetatable(typ))
	return
}

func serveRequest(f filters.FilterContext) func(*lua.LState) int {
	return func(s *lua.LState) int {
		t := s.Get(-1)
		r, ok := t.(*lua.LTable)
		if !ok {
			// TODO(sszuecs): https://github.com/zalando/skipper/issues/1487
			// s.RaiseError("unsupported type %v, need a table", t.Type())
			// return 0
			s.Push(lua.LString("invalid type, need a table"))
			return 1
		}
		res := &http.Response{}
		r.ForEach(serveTableWalk(s, res))
		f.Serve(res)
		return 0
	}
}

func serveTableWalk(s *lua.LState, res *http.Response) func(lua.LValue, lua.LValue) {
	return func(k, v lua.LValue) {
		sk, ok := k.(lua.LString)
		if !ok {
			// TODO(sszuecs): https://github.com/zalando/skipper/issues/1487
			// s.RaiseError("unsupported key type %v, need a string", k.Type())
			return
		}
		switch string(sk) {
		case "status_code":
			n, ok := v.(lua.LNumber)
			if !ok {
				// TODO(sszuecs): https://github.com/zalando/skipper/issues/1487
				// s.RaiseError("unsupported status_code type %v, need a number", v.Type())
				return
			}
			res.StatusCode = int(n)

		case "header":
			t, ok := v.(*lua.LTable)
			if !ok {
				// TODO(sszuecs): https://github.com/zalando/skipper/issues/1487
				// s.RaiseError("unsupported header type %v, need a table", v.Type())
				return
			}
			h := make(http.Header)
			t.ForEach(serveHeaderWalk(h))
			res.Header = h

		case "body":
			var body []byte
			var err error
			switch v.Type() {
			case lua.LTString:
				data := string(v.(lua.LString))
				body = []byte(data)
			case lua.LTTable:
				body, err = gjson.Encode(v.(*lua.LTable))
				if err != nil {
					// TODO(sszuecs): https://github.com/zalando/skipper/issues/1487
					// s.RaiseError("%v", err)
					return
				}
			}
			res.Body = ioutil.NopCloser(bytes.NewBuffer(body))
		}
	}
}

func serveHeaderWalk(h http.Header) func(lua.LValue, lua.LValue) {
	return func(k, v lua.LValue) {
		h.Set(k.String(), v.String())
	}
}

func getContextValue(s *lua.LState) int {
	f := s.CheckUserData(1).Value.(filters.FilterContext)
	key := s.ToString(2)
	var ret lua.LValue
	switch key {
	case "request":
		ret = newProperty(s, typeContextRequest, f)
	case "response":
		ret = newProperty(s, typeContextResponse, f)
	case "state_bag":
		ret = newProperty(s, typeContextStateBag, f)
	case "path_param":
		ret = newProperty(s, typeContextPathParam, f)
	case "serve":
		ret = s.NewFunction(serveRequest(f))
	default:
		return 0
	}
	s.Push(ret)
	return 1
}

func getRequestValue(s *lua.LState) int {
	f := s.CheckUserData(1).Value.(filters.FilterContext)
	key := s.ToString(2)
	var ret lua.LValue
	switch key {
	case "header":
		ret = newProperty(s, typeContextRequestHeader, f)
	case "cookie":
		ret = newProperty(s, typeContextRequestCookie, f)
	case "outgoing_host":
		ret = lua.LString(f.OutgoingHost())
	case "backend_url":
		ret = lua.LString(f.BackendUrl())
	case "host":
		ret = lua.LString(f.Request().Host)
	case "remote_addr":
		ret = lua.LString(f.Request().RemoteAddr)
	case "content_length":
		ret = lua.LNumber(f.Request().ContentLength)
	case "proto":
		ret = lua.LString(f.Request().Proto)
	case "method":
		ret = lua.LString(f.Request().Method)
	case "url":
		ret = lua.LString(f.Request().URL.String())
	case "url_path":
		ret = lua.LString(f.Request().URL.Path)
	case "url_query":
		ret = newProperty(s, typeContextRequestUrlQuery, f)
	case "url_raw_query":
		ret = lua.LString(f.Request().URL.RawQuery)
	default:
		return 0
	}
	s.Push(ret)
	return 1
}

func setRequestValue(s *lua.LState) int {
	f := s.CheckUserData(1).Value.(filters.FilterContext)
	key := s.ToString(2)
	switch key {
	case "outgoing_host":
		f.SetOutgoingHost(s.ToString(3))
	case "url":
		u, err := url.Parse(s.ToString(3))
		if err != nil {
			// TODO(sszuecs): https://github.com/zalando/skipper/issues/1487
			// s.RaiseError("%v", err)
			return 0
		}
		f.Request().URL = u
	case "url_path":
		f.Request().URL.Path = s.ToString(3)
	case "url_raw_query":
		f.Request().URL.RawQuery = s.ToString(3)
	default:
		// TODO(sszuecs): https://github.com/zalando/skipper/issues/1487
		// s.RaiseError("unsupported request field %s", key)
		// do nothing for now
	}
	return 0
}

func getResponseValue(s *lua.LState) int {
	f := s.CheckUserData(1).Value.(filters.FilterContext)
	key := s.ToString(2)
	var ret lua.LValue
	switch key {
	case "header":
		ret = newProperty(s, typeContextResponseHeader, f)
	case "status_code":
		ret = lua.LNumber(f.Response().StatusCode)
	default:
		return 0
	}
	s.Push(ret)
	return 1
}

func setResponseValue(s *lua.LState) int {
	f := s.CheckUserData(1).Value.(filters.FilterContext)
	key := s.ToString(2)
	switch key {
	case "status_code":
		v := s.Get(3)
		n, ok := v.(lua.LNumber)
		if !ok {
			s.RaiseError("unsupported status_code type %v, need a number", v.Type())
			return 0
		}
		f.Response().StatusCode = int(n)
	default:
		s.RaiseError("unsupported response field %s", key)
	}
	return 0
}

func getStateBag(s *lua.LState) int {
	f := s.CheckUserData(1).Value.(filters.FilterContext)
	key := s.ToString(2)
	res, ok := f.StateBag()[key]
	if !ok {
		return 0
	}
	switch res := res.(type) {
	case string:
		s.Push(lua.LString(res))
	case int:
		s.Push(lua.LNumber(res))
	case int64:
		s.Push(lua.LNumber(res))
	case float64:
		s.Push(lua.LNumber(res))
	default:
		return 0
	}
	return 1
}

func setStateBag(s *lua.LState) int {
	f := s.CheckUserData(1).Value.(filters.FilterContext)
	key := s.ToString(2)
	val := s.Get(3)
	var res interface{}
	switch val.Type() {
	case lua.LTString:
		res = string(val.(lua.LString))
	case lua.LTNumber:
		res = float64(val.(lua.LNumber))
	default:
		// TODO(sszuecs): https://github.com/zalando/skipper/issues/1487
		// s.RaiseError("unsupported state bag value type %v, need a string or a number", val.Type())
		return 0
	}
	f.StateBag()[key] = res
	return 0
}

func getRequestHeader(s *lua.LState) int {
	f := s.CheckUserData(1).Value.(filters.FilterContext)
	hdr := s.ToString(2)
	res := f.Request().Header.Get(hdr)
	// TODO(sszuecs): https://github.com/zalando/skipper/issues/1487
	// if res != "" {
	//	s.Push(lua.LString(res))
	//	return 1
	// }
	// return 0
	s.Push(lua.LString(res))
	return 1
}

func setRequestHeader(s *lua.LState) int {
	f := s.CheckUserData(1).Value.(filters.FilterContext)
	hdr := s.ToString(2)
	lv := s.Get(3)
	switch lv.Type() {
	case lua.LTNil:
		f.Request().Header.Del(hdr)
	case lua.LTString:
		str := string(lv.(lua.LString))
		if str == "" {
			f.Request().Header.Del(hdr)
		} else {
			f.Request().Header.Set(hdr, str)
		}
	default:
		val := s.ToString(3)
		f.Request().Header.Set(hdr, val)
	}
	return 0
}

func iterateRequestHeader(s *lua.LState) int {
	// https://www.lua.org/pil/7.2.html
	f := s.CheckUserData(1).Value.(filters.FilterContext)
	s.Push(s.NewFunction(nextHeader(f.Request().Header)))
	return 1
}

func getRequestCookie(s *lua.LState) int {
	f := s.CheckUserData(1).Value.(filters.FilterContext)
	k := s.ToString(2)
	c, err := f.Request().Cookie(k)
	if err == nil {
		s.Push(lua.LString(c.Value))
		return 1
	}
	return 0
}

func iterateRequestCookie(s *lua.LState) int {
	f := s.CheckUserData(1).Value.(filters.FilterContext)
	s.Push(s.NewFunction(nextCookie(f.Request().Cookies())))
	return 1
}

func nextCookie(cookies []*http.Cookie) func(*lua.LState) int {
	return func(s *lua.LState) int {
		if len(cookies) > 0 {
			c := cookies[0]
			s.Push(lua.LString(c.Name))
			s.Push(lua.LString(c.Value))
			cookies[0] = nil // mind peace
			cookies = cookies[1:]
			return 2
		}
		return 0
	}
}

func getResponseHeader(s *lua.LState) int {
	f := s.CheckUserData(1).Value.(filters.FilterContext)
	hdr := s.ToString(2)
	res := f.Response().Header.Get(hdr)
	// TODO(sszuecs): https://github.com/zalando/skipper/issues/1487
	// if res != "" {
	//	s.Push(lua.LString(res))
	//	return 1
	// }
	// return 0
	s.Push(lua.LString(res))
	return 1
}

func setResponseHeader(s *lua.LState) int {
	f := s.CheckUserData(1).Value.(filters.FilterContext)
	hdr := s.ToString(2)
	lv := s.Get(3)
	switch lv.Type() {
	case lua.LTNil:
		f.Response().Header.Del(hdr)
	case lua.LTString:
		str := string(lv.(lua.LString))
		if str == "" {
			f.Response().Header.Del(hdr)
		} else {
			f.Response().Header.Set(hdr, str)
		}
	default:
		val := s.ToString(3)
		f.Response().Header.Set(hdr, val)
	}
	return 0
}

func iterateResponseHeader(s *lua.LState) int {
	f := s.CheckUserData(1).Value.(filters.FilterContext)
	s.Push(s.NewFunction(nextHeader(f.Response().Header)))
	return 1
}

func nextHeader(h http.Header) func(*lua.LState) int {
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	return func(s *lua.LState) int {
		if len(keys) > 0 {
			k := keys[0]
			s.Push(lua.LString(k))
			s.Push(lua.LString(h.Get(k)))
			keys[0] = "" // mind peace
			keys = keys[1:]
			return 2
		}
		return 0
	}
}

func getRequestURLQuery(s *lua.LState) int {
	f := s.CheckUserData(1).Value.(filters.FilterContext)
	k := s.ToString(2)
	res := f.Request().URL.Query().Get(k)
	if res != "" {
		s.Push(lua.LString(res))
		return 1
	}
	return 0
}

func setRequestURLQuery(s *lua.LState) int {
	f := s.CheckUserData(1).Value.(filters.FilterContext)
	k := s.ToString(2)
	lv := s.Get(3)
	q := f.Request().URL.Query()
	switch lv.Type() {
	case lua.LTNil:
		q.Del(k)
	case lua.LTString:
		str := string(lv.(lua.LString))
		if str == "" {
			q.Del(k)
		} else {
			q.Set(k, str)
		}
	default:
		val := s.ToString(3)
		q.Set(k, val)
	}
	f.Request().URL.RawQuery = q.Encode()
	return 0
}

func iterateRequestURLQuery(s *lua.LState) int {
	f := s.CheckUserData(1).Value.(filters.FilterContext)
	s.Push(s.NewFunction(nextQuery(f.Request().URL.Query())))
	return 1
}

func nextQuery(v url.Values) func(*lua.LState) int {
	keys := make([]string, 0, len(v))
	for k := range v {
		keys = append(keys, k)
	}
	return func(s *lua.LState) int {
		if len(keys) > 0 {
			k := keys[0]
			s.Push(lua.LString(k))
			s.Push(lua.LString(v.Get(k)))
			keys[0] = "" // mind peace
			keys = keys[1:]
			return 2
		}
		return 0
	}
}

func getPathParam(s *lua.LState) int {
	f := s.CheckUserData(1).Value.(filters.FilterContext)
	n := s.ToString(2)
	p := f.PathParam(n)
	if p != "" {
		s.Push(lua.LString(p))
		return 1
	}
	return 0
}
