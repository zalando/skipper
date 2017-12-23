
# lua scripts

Scripts for skipper need at least one of two possible functions: `request` or `response`. If
present, they are called with a skipper filter context like
```
function request(ctx)
	print(ctx.request.method .. " " .. ctx.request.url .. " -> " .. ctx.request.backend_url)
end
```

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
