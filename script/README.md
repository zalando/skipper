
# lua scripts

Scripts for skipper need at least one of two possible functions: `request` or `response`. If
present, they are called with a skipper filter context like
```
function request(ctx)
	print(ctx.request.method .. " " .. ctx.request.url .. " -> " .. ctx.request.backend_url)
end
```


The following modules have been preloaded and can be used with e.g.
`local http = require("http")`, see also the examples below

* `http`        "github.com/cjoudrey/gluahttp"
* `url`        "github.com/cjoudrey/gluaurl"
* `json`       "layeh.com/gopher-json" / "github.com/layeh/gopher-json"
* `base64`     "github.com/zalando/skipper/base64"

# Request

## Headers

Request headers can be accessed by accessing the `ctx.request.header` map like
```lua
	ua = ctx.request.header["user-agent"]
```
Header names are normalized by the `net/http` go module like usual. Setting header is done
by assigning to the headers map. Setting a header to `nil` deletes the header:

```lua
	ctx.request.header["user-agent"] = "skipper.lua/0.1"
	ctx.request.header["Authorization"] = nil -- delete authorization header
```

Response headers work the same way by accessing / assigning to `ctx.response.header` - this is of
course only valid in the `response()` phase.

## Other request fields

* `backend_url` - (read only) returns the backend url specified in the route or an empty value in case it's a shunt or loopback
* `outgoing_host` - (read/write) the host that will be set for the outgoing proxy request as the 'Host' header. 
* `remote_addr` - (read only) the remote host, usually IP:port
* `content_length` - (read only) content lenght
* `proto` - (read only) something like "HTTP/1.1"
* `method` - (read only) request method, e.g. "GET" or "POST"
* `url` - (read/write) request URL as string


# Examples

## OAuth2 token as basic auth password

```lua
local base64 = require("base64")

function request(ctx)
        token = string.gsub(ctx.request.header["Authorization"], "^%s*[Bb]earer%s+", "", 1)
        ctx.request.header["Authorization"] = "Basic " .. base64.encode("username:" .. token)
        -- print(ctx.request.header["Authorization"])
end
```

## strip query
```lua
function request(ctx)
        u = ctx.request.url
        s, e = string.find(u, "?")
        if s ~= nil then
                u = string.sub(u, 0, s-1)
		ctx.request.url = u
        end
        -- print("URL="..ctx.request.url)
end
```
