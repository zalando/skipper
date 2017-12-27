# lua scripts

[LUA](https://www.lua.org/)-scripts can be used as filters in skipper. The
current implementation supports [Lua 5.1](https://www.lua.org/manual/5.1/).

A script can be given as file (ending with `.lua`) or as inline code. The
script needs to be the first parameter for the `lua` filter, e.g.
* file `lua("/path/to/file.lua")`
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
ctx.request.header["user-agent"] = "skipper.lua/0.1"
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
