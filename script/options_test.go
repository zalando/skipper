package script

func ExampleLuaOptions_default() {
	runExampleWithOptions(
		LuaOptions{},
		testScript(`
			local base64 = require("base64")

			function request(ctx, params)
				print(table.concat({"Hello", "World"}, " "))
				print(string.lower("Hello World"))
				print(math.abs(-1))
				print(base64.encode("Hello World"))
			end
		`),
	)
	// Output:
	// Hello World
	// hello world
	// 1
	// SGVsbG8gV29ybGQ=
}

const printTable = `
function printTable(p, t)
	local g = {}
	for n in pairs(t) do table.insert(g, n) end
	table.sort(g)
	for i, n in ipairs(g) do print(p..n) end
end`

func ExampleLuaOptions_printGlobals() {
	runExampleWithOptions(
		LuaOptions{},
		testScript(printTable+`
			function request()
				printTable("", _G)
			end
		`),
	)
	// Output:
	// _G
	// _GOPHER_LUA_VERSION
	// _VERSION
	// _printregs
	// assert
	// channel
	// collectgarbage
	// coroutine
	// debug
	// dofile
	// error
	// getfenv
	// getmetatable
	// io
	// ipairs
	// load
	// loadfile
	// loadstring
	// math
	// module
	// newproxy
	// next
	// os
	// package
	// pairs
	// pcall
	// print
	// printTable
	// rawequal
	// rawget
	// rawset
	// request
	// require
	// select
	// setfenv
	// setmetatable
	// sleep
	// string
	// table
	// tonumber
	// tostring
	// type
	// unpack
	// xpcall
}

func ExampleLuaOptions_enableModules() {
	runExampleWithOptions(
		LuaOptions{
			Modules: []string{
				"base._G", "base.pairs", "base.ipairs", "base.print", "base.require",
				"table.sort", "table.insert",
				// enable all symbols from "package" module as
				// additional preloaded modules require it
				"package",
				// preload additional module
				"base64",
			},
		},
		testScript(printTable+`
			local base64 = require("base64")

			function request()
				printTable("", _G)
				printTable("table.", table)
				printTable("package.", package)
				printTable("package.preload.", package.preload)
			end
		`),
	)
	// Output:
	// _G
	// ipairs
	// package
	// pairs
	// print
	// printTable
	// request
	// require
	// table
	// table.insert
	// table.sort
	// package.config
	// package.cpath
	// package.loaded
	// package.loaders
	// package.loadlib
	// package.path
	// package.preload
	// package.seeall
	// package.preload.base64
}

func ExampleLuaOptions_disableAll() {
	runExampleWithOptions(
		LuaOptions{
			// use non-existing module
			Modules: []string{"none"},
		},
		testScript(`function request() print("test") end`),
	)
	// Output:
	// Error calling request from function request() print("test") end: <script>:1: attempt to call a non-function object
	// stack traceback:
	// 	<script>:1: in main chunk
	// 	[G]: ?
}
