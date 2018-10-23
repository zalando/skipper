package apiusagemonitoring

import (
	"encoding/json"
	"fmt"
)

func toTypedJsonOrErr(value interface{}) string {
	var js string
	if jsBytes, err := json.Marshal(value); err == nil {
		js = string(jsBytes)
	} else {
		js = fmt.Sprintf("<%s>", err)
	}
	return fmt.Sprintf("%T %s", value, js)
}
