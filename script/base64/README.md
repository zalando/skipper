# Base64

base64 provides an easy way to encode and decode base64 strings from within
[GopherLua](https://github.com/yuin/gopher-lua)

# Installation

    go get github.com/zalando/skipper/script/base64

# Usage
```go
package main

import "github.com/yuin/gopher-lua"
import "github.com/zalando/skipper/script/base64"

var script string = `
local base64 = require("base64")

data, err = base64.decode("dXNlcjpwYXNzd29yZA==")
if err ~= nil then
    error(err)
else
    print("DATA="..data)
end
print("ENC="..base64.encode(data))
`


func main() {
    L := lua.NewState()
    defer L.Close()

    L.PreloadModule("base64", base64.Loader)

    if err := L.DoString(script); err != nil {
        panic(err)
    }
}
```

# API

## `base64.decode(str)`

Decodes the base64 encoded string. Returns the decoded string or (`nil`, error message).

## `base64.encode(str)`

Returns the base64 encoded str.
