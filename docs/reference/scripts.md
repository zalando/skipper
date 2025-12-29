# Lua filter scripts

[LUA](https://www.lua.org/) scripts can be used as filters in skipper. The
current implementation supports [Lua 5.1](https://www.lua.org/manual/5.1/).

## Route filters

The lua scripts can be added to a route description with the `lua()` filter,
the first parameter for the filter is the script. This can be either a file
name (ending with `.lua`) or inline code, e.g. as

* file `lua("/path/to/file.lua")` - if a file path is not absolute, the path
 is relative to skipper's working directory.
* inline `lua("function request(c, p); print(c.request.url); end")`

Any other additional parameters for the filter will be passed as
a second table parameter to the called functions.
> Any parameter starting with "lua-" should not be used to pass
values for the script - those will be used for configuring the filter.

## Script requirements

A filter script needs at least one global function: `request` or `response`.
If present, they are called with a skipper filter context and the params passed
in the route as table like
```lua
-- route looks like
--
-- any: * -> lua("./test.lua", "myparam=foo", "other=bar", "justkey") -> <shunt>
--
function request(ctx, params)
    print(params[1])      -- myparam=foo
    print(params[2])      -- other=bar
    print(params[3])      -- justkey
    print(params[4])      -- nil
    print(params.myparam) -- foo
    print(params.other)   -- bar
    print(params.justkey) -- (empty string)
    print(params.x)       -- nil
end
```
> Parameter table allows index access as well as key-value access

## print builtin

Lua `print` builtin function writes skipper info log messages.

## sleep

`sleep(number)` function pauses execution for at least `number` milliseconds. A negative or zero duration causes `sleep` to return immediately.

## Enable and Disable lua sources

The flag `-lua-sources` allows to set 5 different values:

* "file": Allows to use reference to file for scripts
* "inline": Allows to use inline scripts
* "inline", "file": Allows to use reference to file and inline scripts
* "none": Disable Lua filters
* "": the same as "inline", "file", the default value for binary and
  library users

## Available lua modules

Besides the [standard modules](https://www.lua.org/manual/5.1/manual.html#5) - except
for `debug` - the following additional modules have been preloaded and can be used with e.g.
`local http = require("http")`, see also the examples below

* `http` [gluahttp](https://github.com/cjoudrey/gluahttp) - TODO: configurable
 with something different than `&http.Client{}`
* `url`  [gluaurl](https://github.com/cjoudrey/gluaurl)
* `json` [gopher-json](https://github.com/layeh/gopher-json)
* `base64` [lua base64](https://github.com/zalando/skipper/tree/master/script/base64)

For differences between the standard modules and the gopher-lua implementation
check the [gopher-lua documentation](https://github.com/yuin/gopher-lua#differences-between-lua-and-gopherlua).

Any other module can be loaded in non-byte code form from the lua path (by default
for `require("mod")` this is `./mod.lua`, `/usr/local/share/lua/5.1/mod.lua` and
`/usr/local/share/lua/5.1/mod/init.lua`).


You may selectively enable standard and additional Lua modules using `-lua-modules` flag:
```sh
-lua-modules=package,base,json
```
Note that preloaded additional modules require `package` module.

For standard modules you may enable only a subset of module symbols:
```sh
-lua-modules=base.print,base.assert
```

Use `none` to disable all modules:
```sh
-lua-modules=none
```

See also http://lua-users.org/wiki/SandBoxes

## Lua states

There is no guarantee that the `request()` and `response()` functions of a
lua script run in the same lua state during one request. Setting a variable
in the request and accessing it in the response will most likely fail and lead
to hard debuggable errors. Use the `ctx.state_bag` to propagate values from
`request` to `response` - and any other filter in the chain.

# Request and response

The `request()` function is run for an incoming request and `response()` for backend response.

## Headers

Request headers can be accessed via `ctx.request.header` table like
```lua
ua = ctx.request.header["user-agent"]
```
and iterated like
```lua
for k, v in ctx.request.header() do
    print(k, "=", v);
end
```
> Header table is a [functable](http://lua-users.org/wiki/FuncTables) that returns [iterator](https://www.lua.org/pil/7.2.html)

Header names are normalized by the `net/http` go module
[like usual](https://pkg.go.dev/net/http#CanonicalHeaderKey). Setting a
header is done by assigning to the header table. Setting a header to `nil` or
an empty string deletes the header - setting to `nil` is preferred.

```lua
ctx.request.header["user-agent"] = "skipper.lua/0.0.1"
ctx.request.header["Authorization"] = nil -- delete authorization header
```
> `header` table returns empty string for missing keys

Response headers `ctx.response.header` work the same way - this is of course only valid in the `response()` phase.

### Multiple header values

Request and response header tables provide access to a first value of a header.

To access multiple values use `add` and `values` methods:

```lua
function request(ctx, params)
	ctx.request.header.add("X-Foo", "Bar")
	ctx.request.header.add("X-Foo", "Baz")

	-- all X-Foo values
	for _, v in pairs(ctx.request.header.values("X-Foo")) do
		print(v)
	end

	-- all values
	for k, _ in ctx.request.header() do
		for _, v in pairs(ctx.request.header.values(k)) do
			print(k, "=", v)
		end
	end
end
```


## Other request fields

* `backend_url` - (read only) returns the backend url specified in the route
  or an empty value if it's a shunt or loopback
* `host` - (read only) the 'Host' header that was in the incoming
  request to the proxy
* `outgoing_host` - (read/write) the host that will be set for the outgoing
  proxy request as the 'Host' header.
* `remote_addr` - (read only) the remote host, usually IP:port
* `content_length` - (read only) content length
* `proto` - (read only) something like "HTTP/1.1"
* `method` - (read only) request method, e.g. "GET" or "POST"
* `url` - (read/write) request URL as string
* `url_path` - (read/write) request URL path as string
* `url_query` - (read/write) request URL query parameter table, similar to header table but returns `nil` for missing keys
* `url_raw_query` - (read/write) encoded request URL query values, without '?' as string
* `cookie` - (read only) request cookie table, similar to header table but returns `nil` for missing keys

## Other response fields

* `status_code` - (read/write) response status code as number, e.g. 200

## Serving requests from lua
Requests can be served with `ctx.serve(table)`, you must return after this
call. Possible keys for the table:

  * `status_code` (number) - required (but currently not enforced)
  * `header` (table)
  * `body` (string)

See also [redirect](#redirect) and [internal server error](#internal-server-error)
examples below

## Path parameters

Path parameters (if any) can be read via `ctx.path_param` table
```
Path("/api/:id") -> lua("function request(ctx, params); print(ctx.path_param.id); end") -> <shunt>
```
> `path_param` table returns `nil` for missing keys

## StateBag

The state bag can be used to pass string, number and table values from one filter to another in the same
chain. It is shared by all filters in one request (lua table values are only available to lua filters).
```lua
function request(ctx, params)
    -- the value of "mykey" will be available to all filters in the chain now:
    ctx.state_bag["mykey"] = "foo"
end

function response(ctx, params)
    print(ctx.state_bag["mykey"])
end
```
> `state_bag` table returns `nil` for missing keys

# Examples

>The examples serve as examples. If there is a go based plugin available,
use that instead. For overhead estimate see [benchmark](#benchmark).

## OAuth2 token as basic auth password
```lua
local base64 = require("base64")

function request(ctx, params)
    token = string.gsub(ctx.request.header["Authorization"], "^%s*[Bb]earer%s+", "", 1)
    user = ctx.request.header["x-username"]
    if user == "" then
        user = params.username
    end
    ctx.request.header["Authorization"] = "Basic " .. base64.encode(user .. ":"  .. token)
    -- print(ctx.request.header["Authorization"])
end
```

## validate token
```lua
local http = require("http")
function request(ctx, params)
    token = string.gsub(ctx.request.header["Authorization"], "^%s*[Bb]earer%s+", "", 1)
    if token == "" then
        ctx.serve({status_code=401, body="Missing Token"})
        return
    end

    res, err = http.get("https://auth.example.com/oauth2/tokeninfo?access_token="..token)
    if err ~= nil then
        print("Failed to get tokeninfo: " .. err)
        ctx.serve({status_code=401, body="Failed to validate token: "..err})
        return
    end
    if res.status_code ~= 200 then
        ctx.serve({status_code=401, body="Invalid token"})
        return
    end
end
```

## strip query
```lua
function request(ctx, params)
    ctx.request.url = string.gsub(ctx.request.url, "%?.*$", "")
    -- print("URL="..ctx.request.url)
end
```

## redirect
```lua
function request(ctx, params)
    ctx.serve({
        status_code=302,
        header={
            location="http://www.example.org/",
        },
    })
end
```

## internal server error
```lua
function request(ctx, params)
    -- let 10% of all requests fail with 500
    if math.random() < 0.1 then
        ctx.serve({
            status_code=500,
            body="Internal Server Error.\n",
        })
    end
end
```

## set request header from params
```lua
function request(ctx, params)
	ctx.request.header[params[1]] = params[2]
	if params[1]:lower() == "host" then
		ctx.request.outgoing_host = params[2]
	end
end
```

# Benchmark

## redirectTo vs lua redirect
See skptesting/benchmark-lua.sh

Route for "skipper" is `* -> redirectTo(302, "http://localhost:9980") -> <shunt>`,
route for "lua" is `* -> lua("function request(c,p); c.serve({status_code=302, header={location='http://localhost:9980'}});end") -> <shunt>`

Benchmark results
```
[benchmarking skipper-redirectTo]
Running 12s test @ http://127.0.0.1:9990/lorem.html
  2 threads and 128 connections
  Thread Stats   Avg      Stdev     Max   +/- Stdev
    Latency     4.19ms    5.38ms  69.50ms   85.10%
    Req/Sec    26.16k     2.63k   33.22k    64.58%
  Latency Distribution
     50%    1.85ms
     75%    6.38ms
     90%   11.66ms
     99%   23.34ms
  626122 requests in 12.04s, 91.36MB read
Requests/sec:  51996.22
Transfer/sec:      7.59MB
[benchmarking skipper-redirectTo done]

[benchmarking redirect-lua]
Running 12s test @ http://127.0.0.1:9991/lorem.html
  2 threads and 128 connections
  Thread Stats   Avg      Stdev     Max   +/- Stdev
    Latency     6.81ms    9.69ms 122.19ms   85.95%
    Req/Sec    21.17k     2.83k   30.63k    73.75%
  Latency Distribution
     50%    2.21ms
     75%   10.22ms
     90%   19.88ms
     99%   42.54ms
  507434 requests in 12.06s, 68.72MB read
Requests/sec:  42064.69
Transfer/sec:      5.70MB
[benchmarking redirect-lua done]
```
show lua performance is ~80% of native.

The benchmark was run with the default pool size of `script.InitialPoolSize = 3; script.MaxPoolSize = 10`.
With `script.InitialPoolSize = 128; script.MaxPoolSize = 128` (tweaked for this benchmark) you get >95% of native performance in lua:
```
[benchmarking skipper-redirectTo]
Running 12s test @ http://127.0.0.1:9990/lorem.html
  2 threads and 128 connections
  Thread Stats   Avg      Stdev     Max   +/- Stdev
    Latency     4.15ms    5.24ms  62.27ms   84.88%
    Req/Sec    25.81k     2.64k   32.74k    70.00%
  Latency Distribution
     50%    1.88ms
     75%    6.49ms
     90%   11.43ms
     99%   22.49ms
  617499 requests in 12.03s, 90.10MB read
Requests/sec:  51336.87
Transfer/sec:      7.49MB
[benchmarking skipper-redirectTo done]

[benchmarking redirect-lua]
Running 12s test @ http://127.0.0.1:9991/lorem.html
  2 threads and 128 connections
  Thread Stats   Avg      Stdev     Max   +/- Stdev
    Latency     3.79ms    4.98ms  91.19ms   87.15%
    Req/Sec    25.14k     4.71k   51.45k    72.38%
  Latency Distribution
     50%    1.61ms
     75%    5.17ms
     90%   10.05ms
     99%   21.83ms
  602630 requests in 12.10s, 81.61MB read
Requests/sec:  49811.24
Transfer/sec:      6.75MB
[benchmarking redirect-lua done]
```

Similar results are achieved when testing `stripQuery()` vs the lua version from above.
