package script

import (
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	lua "github.com/yuin/gopher-lua"
)

type luaModule struct {
	name   string
	loader lua.LGFunction

	disabledSymbols []string
}

var standardModules = []luaModule{
	// Load Package and Base first, see lua.LState.OpenLibs()
	{lua.LoadLibName, lua.OpenPackage, nil},
	{lua.BaseLibName, lua.OpenBase, nil},
	{lua.TabLibName, lua.OpenTable, nil},
	{lua.IoLibName, lua.OpenIo, nil},
	{lua.OsLibName, lua.OpenOs, nil},
	{lua.StringLibName, lua.OpenString, nil},
	{lua.MathLibName, lua.OpenMath, nil},
	{lua.DebugLibName, lua.OpenDebug, nil},
	{lua.ChannelLibName, lua.OpenChannel, nil},
	{lua.CoroutineLibName, lua.OpenCoroutine, nil},
}

// load loads standard lua module, see lua.LState.OpenLibs()
func (m luaModule) load(L *lua.LState) {
	L.Push(L.NewFunction(m.loader))
	L.Push(lua.LString(m.name))
	L.Call(1, 0)

	if m.name == lua.BaseLibName {
		L.SetGlobal("print", L.NewFunction(printToLog))
		L.SetGlobal("sleep", L.NewFunction(sleep))
	}

	if len(m.disabledSymbols) > 0 {
		st := m.table(L)
		for _, name := range m.disabledSymbols {
			st.RawSetString(name, lua.LNil)
		}
	}
}

// withSymbols returns copy of module with selected symbols
func (m luaModule) withSymbols(L *lua.LState, enabledSymbols []string) luaModule {
	// gopher-lua does not have API to select enabled symbols,
	// see https://github.com/yuin/gopher-lua/discussions/393
	//
	// Instead collect symbols to disable as difference
	// between all and enabled module symbols
	allSymbols := make(map[string]struct{})

	m.load(L)
	m.table(L).ForEach(func(k, _ lua.LValue) {
		if name, ok := k.(lua.LString); ok {
			allSymbols[name.String()] = struct{}{}
		}
	})

	for _, s := range enabledSymbols {
		delete(allSymbols, s)
	}

	result := luaModule{m.name, m.loader, nil}
	for s := range allSymbols {
		result.disabledSymbols = append(result.disabledSymbols, s)
	}
	return result
}

func (m luaModule) table(L *lua.LState) *lua.LTable {
	name := m.name
	if m.name == lua.BaseLibName {
		name = "_G"
	}
	return L.GetGlobal(name).(*lua.LTable)
}

func (m luaModule) preload(L *lua.LState) {
	L.PreloadModule(m.name, m.loader)
}

func printToLog(L *lua.LState) int {
	top := L.GetTop()
	args := make([]any, 0, top)
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

func moduleConfig(modules []string) map[string][]string {
	config := make(map[string][]string)
	for _, m := range modules {
		if module, symbol, found := strings.Cut(m, "."); found {
			config[module] = append(config[module], symbol)
		} else {
			if _, ok := config[module]; !ok {
				config[module] = []string{}
			}
		}
	}
	return config
}
