package base64

import (
	"encoding/base64"

	lua "github.com/yuin/gopher-lua"
)

func Loader(L *lua.LState) int {
	mod := L.SetFuncs(L.NewTable(), map[string]lua.LGFunction{
		"decode": decode,
		"encode": encode,
	})
	L.Push(mod)
	return 1
}

func encode(L *lua.LState) int {
	str := L.CheckString(1)
	ret := base64.StdEncoding.EncodeToString([]byte(str))
	L.Push(lua.LString(ret))
	return 1
}

func decode(L *lua.LState) int {
	str := L.CheckString(1)
	ret, err := base64.StdEncoding.DecodeString(str)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	L.Push(lua.LString(ret))
	return 1
}
