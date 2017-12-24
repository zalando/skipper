# Usage
```go
package main

import "github.com/yuin/gopher-lua"
import "github.com/zalando/skipper/script/base64"

func main() {
    L := lua.NewState()
    defer L.Close()

    L.PreloadModule("base64", base64.Loader)

    err := L.DoString(`
        local base64 = require("base64")

        data, err = base64.decode("dXNlcjpwYXNzd29yZA==")
	if err ~= nil then
		error(err)
	else
		print("DATA="..data)
	end
	print("ENC="..base64.encode(data))
    `)
	if err != nil {
        panic(err)
    }
}
```

