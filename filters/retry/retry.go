package retry

import (
	"fmt"
	"net/http"

	"github.com/zalando/skipper/filters"
	"gopkg.in/yaml.v2"
)

const (
	single = "single"
)

type (
	retrySpec   struct{}
	RetryFilter struct {
		Type        string `json:"type,omitempty"`
		StatusCodes []int  `json:"status-codes,omitempty"`
		MaxTimes    int    `json:"max-times,omitempty"`

		Check func(*http.Response) bool
	}
)

// NewRetry creates a filter specification for the retry() filter
func NewRetry() filters.Spec { return &retrySpec{} }

func (*retrySpec) Name() string { return filters.RetryName }

func (s *retrySpec) CreateFilter(args []interface{}) (filters.Filter, error) {
	rf := &RetryFilter{}

	if config, ok := args[0].(string); !ok {
		return nil, fmt.Errorf("filter %q requires single string argument", s.Name())
	} else if err := yaml.Unmarshal([]byte(config), rf); err != nil {
		return nil, fmt.Errorf("failed to parse configuration: %w", err)
	}

	switch rf.Type {
	case single:
		i := 0
		rf.Check = func(rsp *http.Response) bool {
			i++
			if i > rf.MaxTimes {
				return false
			}
			return shouldRetry(rsp.StatusCode, rf.StatusCodes)
		}
	}

	return rf, nil
}

// copy from proxy.shouldLog
func shouldRetry(statusCode int, prefixes []int) bool {
	if len(prefixes) == 0 {
		return false
	}

	match := false
	for _, prefix := range prefixes {
		switch {
		case prefix < 10:
			match = (statusCode >= prefix*100 && statusCode < (prefix+1)*100)
		case prefix < 100:
			match = (statusCode >= prefix*10 && statusCode < (prefix+1)*10)
		default:
			match = statusCode == prefix
		}
		if match {
			break
		}
	}
	return match
}

func (rf *RetryFilter) Response(filters.FilterContext) {}

func (rf *RetryFilter) Request(ctx filters.FilterContext) {
	ctx.StateBag()[filters.RetryName] = rf.Check
}
