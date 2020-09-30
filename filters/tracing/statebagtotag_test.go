package tracing

import (
	"net/http"
	"testing"

	"github.com/opentracing/opentracing-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/filtertest"
	"github.com/zalando/skipper/oauth"
	"github.com/zalando/skipper/tracing/tracingtest"
)

func TestStateBagToTag(t *testing.T) {
	for _, ti := range []struct {
		msg         string
		stateBag    map[string]interface{}
		item        string
		maskedUser  string
		expectedTag string
	}{
		{
			msg:         "non-auth",
			stateBag:    map[string]interface{}{"item": "val"},
			item:        "item",
			expectedTag: "val",
		}, {
			msg:         "auth",
			stateBag:    map[string]interface{}{"auth-user": "val"},
			item:        "auth-user",
			expectedTag: "val",
		}, {
			msg:         "masked auth",
			stateBag:    map[string]interface{}{},
			item:        "auth-user",
			maskedUser:  "masked",
			expectedTag: "masked",
		},
	} {
		t.Run(ti.msg, func(t *testing.T) {
			req := &http.Request{Header: http.Header{}}

			span := tracingtest.NewSpan("start_span")
			req = req.WithContext(opentracing.ContextWithSpan(req.Context(), span))
			ctx := &filtertest.Context{FRequest: req, FStateBag: make(map[string]interface{})}
			for k, v := range ti.stateBag {
				ctx.StateBag()[k] = v
			}

			f, err := NewStateBagToTag([]oauth.MaskOAuthUser{func(stateBag map[string]interface{}) (string, bool) {
				if ti.maskedUser != "" {
					return ti.maskedUser, true
				}
				return "", false
			}}).CreateFilter([]interface{}{ti.item, "tag"})
			require.NoError(t, err)

			f.Request(ctx)

			assert.Equal(t, ti.expectedTag, span.Tags["tag"])
		})
	}
}

func TestStateBagToTag_CreateFilter(t *testing.T) {
	for _, ti := range []struct {
		msg      string
		args     []interface{}
		stateBag string
		tag      string
		err      error
	}{
		{
			msg:      "state bag and tag",
			args:     []interface{}{"state_bag", "tag"},
			stateBag: "state_bag",
			tag:      "tag",
		},
		{
			msg:      "only state bag",
			args:     []interface{}{"state_bag"},
			stateBag: "state_bag",
			tag:      "state_bag",
		},
		{
			msg:  "no args",
			args: []interface{}{},
			err:  filters.ErrInvalidFilterParameters,
		},
		{
			msg:  "empty arg",
			args: []interface{}{""},
			err:  filters.ErrInvalidFilterParameters,
		},
	} {
		t.Run(ti.msg, func(t *testing.T) {
			f, err := NewStateBagToTag(nil).CreateFilter(ti.args)

			assert.Equal(t, ti.err, err)
			if err == nil {
				ff := f.(stateBagToTagFilter)

				assert.Equal(t, ti.stateBag, ff.stateBagItemName)
				assert.Equal(t, ti.tag, ff.tagName)
			}
		})
	}
}
