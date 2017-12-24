// Package script provides lua scripting for skipper
package script

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"

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
	return "luaScript"
}

func (ls *luaScript) CreateFilter(config []interface{}) (filters.Filter, error) {
	if len(config) == 0 {
		return nil, filters.ErrInvalidFilterParameters
	}
	src, ok := config[0].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}

	l := lua.NewState()
	l.PreloadModule("base64", base64.Loader)
	l.PreloadModule("http", gluahttp.NewHttpModule(&http.Client{}).Loader)
	l.PreloadModule("url", gluaurl.Loader)
	l.PreloadModule("json", gjson.Loader)

	var err error
	if strings.HasSuffix(src, ".lua") {
		err = l.DoFile(src)
	} else {
		err = l.DoString(src)
	}
	if err != nil {
		log.Printf("ERROR loading `%s`: %s", src, err)
		l.Close()
		return nil, filters.ErrInvalidFilterParameters
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
		return nil, filters.ErrInvalidFilterParameters
	}

	pt := l.NewTable()
	for _, p := range config[1:] {
		ps, ok := p.(string)
		if !ok {
			l.Close()
			return nil, filters.ErrInvalidFilterParameters
		}
		parts := strings.SplitN(ps, "=", 2)
		if len(parts) == 1 {
			parts = append(parts, "")
		}
		pt.RawSetString(parts[0], lua.LString(parts[1]))
	}
	return &script{state: l, source: src, params: pt}, nil
}

type script struct {
	state  *lua.LState
	source string
	params *lua.LTable
}

func (s *script) Request(f filters.FilterContext) {
	s.runFunc("request", f)
}

func (s *script) Response(f filters.FilterContext) {
	s.runFunc("response", f)
}

func (s *script) runFunc(name string, f filters.FilterContext) {
	fn := s.state.GetGlobal(name)
	if fn.Type() != lua.LTFunction {
		return
	}

	err := s.state.CallByParam(
		lua.P{
			Fn:      fn,
			NRet:    0,
			Protect: true,
		},
		s.filterContextAsLuaTable(f),
		s.params,
	)
	if err != nil {
		fmt.Printf("Error calling %s from %s: %s", name, s.source, err)
	}
}

func (s *script) filterContextAsLuaTable(f filters.FilterContext) *lua.LTable {
	// this will be passed as parameter to the lua functions
	t := s.state.NewTable()

	// access to f.Request():
	req := s.state.NewTable()
	t.RawSet(lua.LString("request"), req)

	// add metatable to dynamically access fields in the request
	req_mt := s.state.NewTable()
	req_mt.RawSet(lua.LString("__index"), s.state.NewFunction(getRequestValue(f)))
	req_mt.RawSet(lua.LString("__newindex"), s.state.NewFunction(setRequestValue(f)))
	s.state.SetMetatable(req, req_mt)

	// and the request headers
	reqhdr := s.state.NewTable()
	reqhdr_mt := s.state.NewTable()
	reqhdr_mt.RawSet(lua.LString("__index"), s.state.NewFunction(getRequestHeader(f)))
	reqhdr_mt.RawSet(lua.LString("__newindex"), s.state.NewFunction(setRequestHeader(f)))
	req.RawSet(lua.LString("header"), reqhdr)
	s.state.SetMetatable(reqhdr, reqhdr_mt)

	// same for response, a bit simpler
	res := s.state.NewTable()
	reshdr := s.state.NewTable()
	reshdr_mt := s.state.NewTable()
	reshdr_mt.RawSet(lua.LString("__index"), s.state.NewFunction(getResponseHeader(f)))
	reshdr_mt.RawSet(lua.LString("__newindex"), s.state.NewFunction(setResponseHeader(f)))
	s.state.SetMetatable(reshdr, reshdr_mt)
	res.RawSet(lua.LString("header"), reshdr)
	t.RawSet(lua.LString("response"), res)

	// finally
	t.RawSet(lua.LString("serve"), s.state.NewFunction(serveRequest(f)))

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
