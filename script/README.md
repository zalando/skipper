# lua scripts

[LUA](https://www.lua.org/)-scripts can be used as filters in skipper. The
current implementation supports [Lua 5.1](https://www.lua.org/manual/5.1/).

A script can be given as file (ending with `.lua`) or as inline code. The
script needs to be the first parameter for the `lua` filter, e.g.
* file `lua("/path/to/file.lua")` - if a file path is not absolute, the path
 is relative to skipper's working directory.
* inline `lua("function request(c, p); print(c.request.url); end")`

Any other additional parameters for the filter must be `key=value` strings.
These will be passed as table to the called functions as second parameter.
**NOTE**: Any parameter starting with "lua-" should not be used to pass
values for the script - those will be used for configuring the filter.

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
The following modules have been preloaded and can be used with e.g.
`local http = require("http")`, see also the examples below
* `http` [gluahttp](https://github.com/cjoudrey/gluahttp) - TODO: configurable
 with something different than `&http.Client{}`
* `url`  [gluaurl](https://github.com/cjoudrey/gluaurl)
* `json` [gopher-json](https://github.com/layeh/gopher-json)
* `base64` [lua base64](./base64/)

There is no guarantee that the `request()` and `response()` functions of a
lua script run in the same lua state during one request. Setting a variable
in the request and accessing it in the response will lead to hard debuggable
errors. Use the `ctx.state_bag` to propagate values from `request` to
`response` - and any other filter in the chain.

# Request

## Headers

Request headers can be accessed by accessing the `ctx.request.header` map like
```lua
ua = ctx.request.header["user-agent"]
```
Header names are normalized by the `net/http` go module like usual. Setting
header is done by assigning to the headers map. Setting a header to `nil` or
an empty string deletes the header - setting to `nil` is preferred.
```lua
ctx.request.header["user-agent"] = "skipper.lua/0.0.1"
ctx.request.header["Authorization"] = nil -- delete authorization header
```

Response headers work the same way by accessing / assigning to
`ctx.response.header` - this is of course only valid in the `response()` phase.

## Other request fields

* `backend_url` - (read only) returns the backend url specified in the route or an empty value in case it's a shunt or loopback
* `outgoing_host` - (read/write) the host that will be set for the outgoing proxy request as the 'Host' header. 
* `remote_addr` - (read only) the remote host, usually IP:port
* `content_length` - (read only) content lenght
* `proto` - (read only) something like "HTTP/1.1"
* `method` - (read only) request method, e.g. "GET" or "POST"
* `url` - (read/write) request URL as string

## Serving requests from lua
* `serve(table)` - table needs `status_code` (number) and `header` (table) keys - more to come :), see redirect example
 below, TODO: add `body`

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
        ctx.serve({status_code=401})
        return
    end
    -- do not use in production, no circuit breaker ...
    res, err = http.get("https://auth.example.com/oauth2/tokeninfo?access_token="..token)
    if err ~= nil then
        print("Failed to get tokeninfo: " .. err)
        ctx.serve({status_code=401})
        return
    end
    if res.status_code ~= 200 then
        ctx.serve({status_code=401})
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

# Benchmark

## redirectTo vs lua redirect
See skptesting/benchmark-lua.sh

Route for "skipper" is `* -> redirectTo(302, "http://localhost:9980") -> <shunt>`,
route for "lua" is `* -> lua("function request(c,p); c.serve({status_code=302, header={location='http://localhost:9980'}});end") -> <shunt>`

```
[benchmarking skipper-redirectTo]
Running 12s test @ http://127.0.0.1:9990/lorem.html
  2 threads and 128 connections
  Thread Stats   Avg      Stdev     Max   +/- Stdev
    Latency     3.91ms    4.87ms  59.40ms   85.80%
    Req/Sec    24.92k     6.05k   36.78k    60.83%
  Latency Distribution
     50%    1.83ms
     75%    6.01ms
     90%   10.37ms
     99%   21.33ms
  596683 requests in 12.04s, 87.07MB read
Requests/sec:  49542.84
Transfer/sec:      7.23MB
[benchmarking skipper-redirectTo done]

[benchmarking redirect-lua]
Running 12s test @ http://127.0.0.1:9991/lorem.html
  2 threads and 128 connections
  Thread Stats   Avg      Stdev     Max   +/- Stdev
    Latency    14.86ms   21.87ms 342.03ms   87.17%
    Req/Sec    10.44k     2.00k   15.07k    67.08%
  Latency Distribution
     50%    4.48ms
     75%   22.31ms
     90%   42.07ms
     99%   98.44ms
  250358 requests in 12.05s, 33.90MB read
Requests/sec:  20775.38
Transfer/sec:      2.81MB
[benchmarking redirect-lua done]
```
The benchmark was run with the default pool size of `script.InitialPoolSize = 3; script.MaxPoolSize = 10`.
With `script.InitialPoolSize = 128; script.MaxPoolSize = 128` (tweaked for this benchmark) you get about 31k req/s in lua:
```
[benchmarking skipper-redirectTo]
Running 12s test @ http://127.0.0.1:9990/lorem.html
  2 threads and 128 connections
  Thread Stats   Avg      Stdev     Max   +/- Stdev
    Latency     3.96ms    4.89ms  78.09ms   85.34%
    Req/Sec    24.45k     3.74k   37.68k    77.92%
  Latency Distribution
     50%    1.78ms
     75%    6.13ms
     90%   10.57ms
     99%   21.11ms
  585192 requests in 12.04s, 85.39MB read
Requests/sec:  48617.36
Transfer/sec:      7.09MB
[benchmarking skipper-redirectTo done]

[benchmarking redirect-lua]
Running 12s test @ http://127.0.0.1:9991/lorem.html
  2 threads and 128 connections
  Thread Stats   Avg      Stdev     Max   +/- Stdev
    Latency     8.25ms   12.37ms 170.95ms   87.93%
    Req/Sec    15.82k     1.96k   22.00k    69.33%
  Latency Distribution
     50%    2.80ms
     75%   10.20ms
     90%   23.99ms
     99%   57.44ms
  378803 requests in 12.05s, 51.30MB read
Requests/sec:  31447.98
Transfer/sec:      4.26MB
[benchmarking redirect-lua done]
```

Similar results are achieved when testing `stripQuery()` vs the lua version from above.
