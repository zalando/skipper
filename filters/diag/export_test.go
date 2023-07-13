package diag

import (
	"fmt"
	"time"

	"github.com/zalando/skipper/filters"
)

func SetSleep(f filters.Filter, sleep func(time.Duration)) {
	switch f := f.(type) {
	case *histFilter:
		f.sleep = sleep
	case *jitter:
		f.sleep = sleep
	default:
		panic(fmt.Sprintf("unsupported filter type: %T", f))
	}
}
