package filters

import (
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/filters/cookie"
	"strconv"
)

func Status(statusCode int) *eskip.Filter {
	return &eskip.Filter{
		Name: builtin.StatusName,
		Args: []interface{}{statusCode},
	}
}

func InlineContent(text string) *eskip.Filter {
	return &eskip.Filter{
		Name: builtin.InlineContentName,
		Args: []interface{}{text},
	}
}

func InlineContentWithMime(text, mime string) *eskip.Filter {
	return &eskip.Filter{
		Name: builtin.InlineContentName,
		Args: []interface{}{text, mime},
	}
}

func PreserveHost(preserve bool) *eskip.Filter {
	return &eskip.Filter{
		Name: builtin.PreserveHostName,
		Args: []interface{}{strconv.FormatBool(preserve)},
	}
}

func ResponseCookie(name, value string) *eskip.Filter {
	return &eskip.Filter{
		Name: cookie.ResponseCookieFilterName,
		Args: []interface{}{name, value},
	}
}

func ResponseCookieWithSettings(name, value string, ttl float64, changeOnly bool) *eskip.Filter {
	return &eskip.Filter{
		Name: cookie.ResponseCookieFilterName,
		Args: []interface{}{name, value, ttl, changeOnly},
	}
}
