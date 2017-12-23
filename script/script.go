package script

import (
	"fmt"
	"github.com/zalando/skipper/script/base64"
	"log"
	"net/url"
	"strings"

	lua "github.com/yuin/gopher-lua"
	"github.com/zalando/skipper/filters"
)

type luaScript struct{}

func NewLuaScript() filters.Spec {
	return &luaScript{}
}

func (ls *luaScript) Name() string {
	return "luaScript"
}

func (ls *luaScript) CreateFilter(config []interface{}) (filters.Filter, error) {
	if len(config) != 1 {
		return nil, filters.ErrInvalidFilterParameters
	}
	src, ok := config[0].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}

	l := lua.NewState()
	l.PreloadModule("base64", base64.Loader)
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
	return &script{state: l, source: src}, nil
}

type script struct {
	state  *lua.LState
	source string
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
	)
	if err != nil {
		fmt.Printf("Error calling %s from %s: %s", name, s.source, err)
	}
}

type luaContext struct {
	filters.FilterContext
}

func (s *script) filterContextAsLuaTable(f filters.FilterContext) *lua.LTable {
	ctx := &luaContext{f}

	t := s.state.NewTable()

	req := s.state.NewTable()
	t.RawSet(lua.LString("request"), req)

	req_mt := s.state.NewTable()
	req_mt.RawSet(lua.LString("__index"), s.state.NewFunction(ctx.getRequestValue))
	req_mt.RawSet(lua.LString("__newindex"), s.state.NewFunction(ctx.setRequestValue))

	s.state.SetMetatable(req, req_mt)

	reqhdr := s.state.NewTable()
	reqhdr_mt := s.state.NewTable()
	reqhdr_mt.RawSet(lua.LString("__index"), s.state.NewFunction(ctx.getRequestHeader))
	reqhdr_mt.RawSet(lua.LString("__newindex"), s.state.NewFunction(ctx.setRequestHeader))
	req.RawSet(lua.LString("header"), reqhdr)
	s.state.SetMetatable(reqhdr, reqhdr_mt)

	res := s.state.NewTable()
	reshdr := s.state.NewTable()
	reshdr_mt := s.state.NewTable()
	reshdr_mt.RawSet(lua.LString("__index"), s.state.NewFunction(ctx.getResponseHeader))
	reshdr_mt.RawSet(lua.LString("__newindex"), s.state.NewFunction(ctx.setResponseHeader))
	s.state.SetMetatable(reshdr, reshdr_mt)
	res.RawSet(lua.LString("header"), reshdr)
	t.RawSet(lua.LString("response"), res)

	return t
}

func (c *luaContext) getRequestValue(s *lua.LState) int {
	key := s.ToString(-1)
	var ret lua.LValue
	switch key {
	case "outgoing_host":
		ret = lua.LString(c.OutgoingHost())
	case "backend_url":
		ret = lua.LString(c.BackendUrl())
	case "remote_addr":
		ret = lua.LString(c.Request().RemoteAddr)
	case "content_length":
		ret = lua.LNumber(c.Request().ContentLength)
	case "proto":
		ret = lua.LString(c.Request().Proto)
	case "method":
		ret = lua.LString(c.Request().Method)
	case "url":
		ret = lua.LString(c.Request().URL.String())
	default:
		ret = lua.LNil
	}
	s.Push(ret)
	return 1
}

func (c *luaContext) setRequestValue(s *lua.LState) int {
	key := s.ToString(-2)
	switch key {
	case "outgoing_host":
		c.SetOutgoingHost(s.ToString(-1))
	case "url":
		u, err := url.Parse(s.ToString(-1))
		if err != nil {
			s.Push(lua.LString(err.Error()))
			return 1
		}
		c.Request().URL = u
	default:
		// do nothing for now
	}
	return 0
}

func (c *luaContext) getRequestHeader(s *lua.LState) int {
	hdr := s.ToString(-1)
	res := c.Request().Header.Get(hdr)
	s.Push(lua.LString(res))
	return 1
}

func (c *luaContext) setRequestHeader(s *lua.LState) int {
	lv := s.Get(-1)
	hdr := s.ToString(-2)
	switch lv.Type() {
	case lua.LTNil:
		c.Request().Header.Del(hdr)
	case lua.LTString:
		str := string(lv.(lua.LString))
		if str == "" {
			c.Request().Header.Del(hdr)
		} else {
			c.Request().Header.Set(hdr, str)
		}
	default:
		val := s.ToString(-1)
		c.Request().Header.Set(hdr, val)
	}
	return 0
}

func (c *luaContext) getResponseHeader(s *lua.LState) int {
	hdr := s.ToString(-1)
	res := c.Response().Header.Get(hdr)
	s.Push(lua.LString(res))
	return 1
}

func (c *luaContext) setResponseHeader(s *lua.LState) int {
	lv := s.Get(-1)
	hdr := s.ToString(-2)
	switch lv.Type() {
	case lua.LTNil:
		c.Response().Header.Del(hdr)
	case lua.LTString:
		str := string(lv.(lua.LString))
		if str == "" {
			c.Response().Header.Del(hdr)
		} else {
			c.Response().Header.Set(hdr, str)
		}
	default:
		val := s.ToString(-1)
		c.Response().Header.Set(hdr, val)
	}
	return 0
}
