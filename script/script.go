// Package script provides lua scripting for skipper
package script

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	lua "github.com/yuin/gopher-lua"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/script/base64"

	"github.com/cjoudrey/gluahttp"
	"github.com/cjoudrey/gluaurl"
	gjson "layeh.com/gopher-json"
)

type luaScript struct{}

func NewLuaScript() filters.Spec {
	return &luaScript{}
}

func (ls *luaScript) Name() string {
	return "lua"
}

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
	s.Lock()
	defer s.Unlock()
	n := len(s.pool)
	if n == 0 {
		s.newState()
	}
	x := s.pool[n-1]
	s.pool = s.pool[0 : n-1]
	return x, nil
}

func (s *script) putState(L *lua.LState) {
	s.Lock()
	defer s.Unlock()
	s.pool = append(s.pool, L)
}

func (s *script) newState() (*lua.LState, error) {
	l := lua.NewState()
	l.PreloadModule("base64", base64.Loader)
	l.PreloadModule("http", gluahttp.NewHttpModule(&http.Client{}).Loader)
	l.PreloadModule("url", gluaurl.Loader)
	l.PreloadModule("json", gjson.Loader)

	var err error
	if strings.HasSuffix(s.source, ".lua") {
		err = l.DoFile(s.source)
	} else {
		err = l.DoString(s.source)
	}
	if err != nil {
		log.Printf("ERROR loading `%s`: %s", s.source, err)
		l.Close()
		return nil, err
	}

	hasFuncs := false
	for _, name := range []string{"request", "response"} {
		fn := l.GetGlobal(name)
		if fn.Type() == lua.LTFunction {
			hasFuncs = true
		}
	}
	if !hasFuncs {
		log.Printf("ERROR: at least one of `request` and `response` function must be present")
		l.Close()
		return nil, errors.New("at least one of `request` and `response` function must be present")
	}

	return l, nil
}

func (s *script) initScript() error {
	s.pool = make([]*lua.LState, 0, 5)
	L, err := s.newState()
	if err != nil {
		return err
	}
	s.putState(L)
	return nil
}

type script struct {
	sync.Mutex
	source      string
	routeParams []string
	pool        []*lua.LState
}

func (s *script) Request(f filters.FilterContext) {
	start := time.Now()
	s.runFunc("request", f)
	log.Printf("request(): took %s", time.Now().Sub(start))
}

func (s *script) Response(f filters.FilterContext) {
	s.runFunc("response", f)
}

func (s *script) runFunc(name string, f filters.FilterContext) {
	L, err := s.getState()
	if err != nil {
		log.Printf("ERROR: %s", err)
		return
	}
	defer s.putState(L)

	fn := L.GetGlobal(name)
	if fn.Type() != lua.LTFunction {
		return
	}

	pt := L.NewTable()
	for _, p := range s.routeParams {
		parts := strings.SplitN(p, "=", 2)
		if len(parts) == 1 {
			parts = append(parts, "")
		}
		pt.RawSetString(parts[0], lua.LString(parts[1]))
	}

	start := time.Now()
	err = L.CallByParam(
		lua.P{
			Fn:      fn,
			NRet:    0,
			Protect: true,
		},
		s.filterContextAsLuaTable(L, f),
		pt,
	)
	if err != nil {
		fmt.Printf("Error calling %s from %s: %s", name, s.source, err)
	}
	log.Printf("running %s: %s", name, time.Now().Sub(start))
}

func (s *script) filterContextAsLuaTable(L *lua.LState, f filters.FilterContext) *lua.LTable {
	// this will be passed as parameter to the lua functions
	t := L.NewTable()

	// access to f.Request():
	req := L.NewTable()
	t.RawSet(lua.LString("request"), req)

	// add metatable to dynamically access fields in the request
	req_mt := L.NewTable()
	req_mt.RawSet(lua.LString("__index"), L.NewFunction(getRequestValue(f)))
	req_mt.RawSet(lua.LString("__newindex"), L.NewFunction(setRequestValue(f)))
	L.SetMetatable(req, req_mt)

	sb := L.NewTable()
	sb_mt := L.NewTable()
	sb_mt.RawSet(lua.LString("__index"), L.NewFunction(getStateBag(f)))
	sb_mt.RawSet(lua.LString("__newindex"), L.NewFunction(setStateBag(f)))
	L.SetMetatable(sb, sb_mt)
	t.RawSet(lua.LString("state_bag"), sb)

	// and the request headers
	reqhdr := L.NewTable()
	reqhdr_mt := L.NewTable()
	reqhdr_mt.RawSet(lua.LString("__index"), L.NewFunction(getRequestHeader(f)))
	reqhdr_mt.RawSet(lua.LString("__newindex"), L.NewFunction(setRequestHeader(f)))
	req.RawSet(lua.LString("header"), reqhdr)
	L.SetMetatable(reqhdr, reqhdr_mt)

	// same for response, a bit simpler
	res := L.NewTable()
	reshdr := L.NewTable()
	reshdr_mt := L.NewTable()
	reshdr_mt.RawSet(lua.LString("__index"), L.NewFunction(getResponseHeader(f)))
	reshdr_mt.RawSet(lua.LString("__newindex"), L.NewFunction(setResponseHeader(f)))
	L.SetMetatable(reshdr, reshdr_mt)
	res.RawSet(lua.LString("header"), reshdr)
	t.RawSet(lua.LString("response"), res)

	// finally
	t.RawSet(lua.LString("serve"), L.NewFunction(serveRequest(f)))

	return t
}

func serveRequest(f filters.FilterContext) func(*lua.LState) int {
	return func(s *lua.LState) int {
		t := s.Get(-1)
		r, ok := t.(*lua.LTable)
		if !ok {
			s.Push(lua.LString("invalid type, need a table"))
			return 1
		}
		res := &http.Response{}
		r.ForEach(serveTableWalk(res))
		f.Serve(res)
		return 0
	}
}

func serveTableWalk(res *http.Response) func(lua.LValue, lua.LValue) {
	return func(k, v lua.LValue) {
		s, ok := k.(lua.LString)
		if !ok {
			return
		}
		switch string(s) {
		case "status_code":
			n, ok := v.(lua.LNumber)
			if !ok {
				return
			}
			res.StatusCode = int(n)
		case "header":
			t, ok := v.(*lua.LTable)
			if !ok {
				return
			}
			h := make(http.Header)
			t.ForEach(serveHeaderWalk(h))
			res.Header = h
		}
	}
}

func serveHeaderWalk(h http.Header) func(lua.LValue, lua.LValue) {
	return func(k, v lua.LValue) {
		h.Set(k.String(), v.String())
	}
}

func getRequestValue(f filters.FilterContext) func(*lua.LState) int {
	return func(s *lua.LState) int {
		key := s.ToString(-1)
		var ret lua.LValue
		switch key {
		case "outgoing_host":
			ret = lua.LString(f.OutgoingHost())
		case "backend_url":
			ret = lua.LString(f.BackendUrl())
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
		default:
			ret = lua.LNil
		}
		s.Push(ret)
		return 1
	}
}

func setRequestValue(f filters.FilterContext) func(*lua.LState) int {
	return func(s *lua.LState) int {
		key := s.ToString(-2)
		switch key {
		case "outgoing_host":
			f.SetOutgoingHost(s.ToString(-1))
		case "url":
			u, err := url.Parse(s.ToString(-1))
			if err != nil {
				s.Push(lua.LString(err.Error()))
				return 1
			}
			f.Request().URL = u
		default:
			// do nothing for now
		}
		return 0
	}
}

func getStateBag(f filters.FilterContext) func(*lua.LState) int {
	return func(s *lua.LState) int {
		fld := s.ToString(-1)
		res, ok := f.StateBag()[fld]
		if !ok {
			s.Push(lua.LNil)
			return 1
		}
		switch res.(type) {
		case string:
			s.Push(lua.LString(res.(string)))
		case int:
			s.Push(lua.LNumber(res.(int)))
		case int64:
			s.Push(lua.LNumber(res.(int64)))
		case float64:
			s.Push(lua.LNumber(res.(float64)))
		default:
			s.Push(lua.LNil)
		}
		return 1
	}
}

func setStateBag(f filters.FilterContext) func(*lua.LState) int {
	return func(s *lua.LState) int {
		fld := s.ToString(-2)
		val := s.Get(-1)
		var res interface{}
		switch val.Type() {
		case lua.LTString:
			res = string(val.(lua.LString))
		case lua.LTNumber:
			res = float64(val.(lua.LNumber))
		default:
			s.Push(lua.LString("unsupported type for state bag"))
			return 1
		}

		f.StateBag()[fld] = res
		return 0
	}
}



func getRequestHeader(f filters.FilterContext) func(*lua.LState) int {
	return func(s *lua.LState) int {
		hdr := s.ToString(-1)
		res := f.Request().Header.Get(hdr)
		s.Push(lua.LString(res))
		return 1
	}
}

func setRequestHeader(f filters.FilterContext) func(*lua.LState) int {
	return func(s *lua.LState) int {
		lv := s.Get(-1)
		hdr := s.ToString(-2)
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
			val := s.ToString(-1)
			f.Request().Header.Set(hdr, val)
		}
		return 0
	}
}

func getResponseHeader(f filters.FilterContext) func(*lua.LState) int {
	return func(s *lua.LState) int {
		hdr := s.ToString(-1)
		res := f.Response().Header.Get(hdr)
		s.Push(lua.LString(res))
		return 1
	}
}

func setResponseHeader(f filters.FilterContext) func(*lua.LState) int {
	return func(s *lua.LState) int {
		lv := s.Get(-1)
		hdr := s.ToString(-2)
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
			val := s.ToString(-1)
			f.Response().Header.Set(hdr, val)
		}
		return 0
	}
}
