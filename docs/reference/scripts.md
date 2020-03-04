# Lua filter scripts

[LUA](https://www.lua.org/) scripts can be used as filters in skipper. The
current implementation supports [Lua 5.1](https://www.lua.org/manual/5.1/).

## Route filters

The lua scripts can be added to a route description with the lua() filter,
the first parameter for the filter is the script. This can be either a file
name (ending with `.lua`) or inline code, e.g. as

* file `lua("/path/to/file.lua")` - if a file path is not absolute, the path
 is relative to skipper's working directory.
* inline `lua("function request(c, p); print(c.request.url); end")`

Any other additional parameters for the filter must be `key=value` strings.
These will be passed as table to the called functions as second parameter.
**NOTE**: Any parameter starting with "lua-" should not be used to pass
values for the script - those will be used for configuring the filter.

## Script requirements

A filter script needs at least one global function: `request` or `response`.
If present, they are called with a skipper filter context and the params passed
in the route as table like
```lua
-- route looks like
--
-- any: * -> lua("./test.lua", "myparam=foo", "other=bar") -> <shunt>
--
function request(ctx, params)
    print(ctx.request.method .. " " .. ctx.request.url .. " -> " .. params.myparam)
end
```

## Available lua modules

Besides the [standard modules](https://www.lua.org/manual/5.1/manual.html#5) - except
for `debug` - the following modules have been preloaded and can be used with e.g.
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

## Lua states

There is no guarantee that the `request()` and `response()` functions of a
lua script run in the same lua state during one request. Setting a variable
in the request and accessing it in the response will most likely fail and lead
to hard debuggable errors. Use the `ctx.state_bag` to propagate values from
`request` to `response` - and any other filter in the chain.

# Request

The `request()` function is run for an incoming request.

## Headers

Request headers can be accessed by accessing the `ctx.request.header` map like
```lua
ua = ctx.request.header["user-agent"]
```

Header names are normalized by the `net/http` go module
[like usual](https://godoc.org/net/http#CanonicalHeaderKey). Setting a
header is done by assigning to the headers map. Setting a header to `nil` or
an empty string deletes the header - setting to `nil` is preferred.

```lua
ctx.request.header["user-agent"] = "skipper.lua/0.0.1"
ctx.request.header["Authorization"] = nil -- delete authorization header
```

Response headers work the same way by accessing / assigning to
`ctx.response.header` - this is of course only valid in the `response()` phase.

## Other request fields

* `backend_url` - (read only) returns the backend url specified in the route
  or an empty value in case it's a shunt or loopback
* `outgoing_host` - (read/write) the host that will be set for the outgoing
  proxy request as the 'Host' header.
* `remote_addr` - (read only) the remote host, usually IP:port
* `content_length` - (read only) content length
* `proto` - (read only) something like "HTTP/1.1"
* `method` - (read only) request method, e.g. "GET" or "POST"
* `url` - (read/write) request URL as string

## Serving requests from lua
Requests can be served with `ctx.serve(table)`, you must return after this
call. Possible keys for the table:

  * `status_code` (number) - required (but currently not enforced)
  * `header` (table)
  * `body` (string)

See also [redirect](#redirect) and [internal server error](#internal-server-error)
examples below

## StateBag

The state bag can be used to pass values from one filter to another in the same
chain. It is shared by all filters in one request.
```lua
function request(ctx, params)
    -- the value of "mykey" will be available to all filters in the chain now:
    ctx.state_bag["mykey"] = "foo"
end

function response(ctx, params)
    print(ctx.state_bag["mykey"])
end
```

# Examples

Note: the examples serve as examples. If there is a go based plugin available,
use that instead. The overhead of calling lua is 4-5 times slower than pure go.

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

# Benchmark

## redirectTo vs lua redirect
See skptesting/benchmark-lua.sh

Route for "skipper" is `* -> redirectTo(302, "http://localhost:9980") -> <shunt>`,
route for "lua" is `* -> lua("function request(c,p); c.serve({status_code=302, header={location='http://localhost:9980'}});end") -> <shunt>`

```
[benchmarking skipper]
Running 12s test @ http://127.0.0.1:9990/lorem.html
  2 threads and 128 connections
  Thread Stats   Avg      Stdev     Max   +/- Stdev
    Latency     6.75ms   14.22ms 260.28ms   92.19%
    Req/Sec    23.87k     2.93k   32.22k    70.42%
  572695 requests in 12.06s, 100.49MB read
  Non-2xx or 3xx responses: 572695
Requests/sec:  47474.31
Transfer/sec:      8.33MB
[benchmarking skipper done]

[benchmarking lua]
Running 12s test @ http://127.0.0.1:9991/lorem.html
  2 threads and 128 connections
  Thread Stats   Avg      Stdev     Max   +/- Stdev
    Latency    38.31ms   53.48ms 580.80ms   83.69%
    Req/Sec     5.44k     1.03k    8.23k    71.25%
  130123 requests in 12.01s, 20.97MB read
Requests/sec:  10831.94
Transfer/sec:      1.75MB
[benchmarking lua done]
```
The benchmark was run with the default pool size of `script.InitialPoolSize = 3; script.MaxPoolSize = 10`.
With `script.InitialPoolSize = 128; script.MaxPoolSize = 128` (tweaked for this benchmark) you get about 12k req/s in lua.

Similar results are achieved when testing `stripQuery()` vs the lua version from above.
