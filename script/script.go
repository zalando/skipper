package script

import (
	"fmt"
	"log"
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

func (s *script) filterContextAsLuaTable(f filters.FilterContext) *lua.LTable {
	t := s.state.NewTable()
	t.RawSetString("request_url", lua.LString(f.Request().URL.String()))
	return t
}
