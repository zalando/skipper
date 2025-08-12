package ratelimit

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/filtertest"
)

func TestLeakyBucketFilterInvalidArgs(t *testing.T) {
	spec := &leakyBucketSpec{
		create: func(_ int, _ time.Duration) leakyBucket {
			t.Fatal("unexpected call to create a bucket")
			return nil
		},
	}
	assert.Equal(t, filters.ClusterLeakyBucketRatelimitName, spec.Name())

	for i, test := range []struct {
		args []interface{}
	}{
		{[]interface{}{"missing args"}},
		{[]interface{}{123, 1, "1s", 1, 1}},
		{[]interface{}{"alabel", "invalid volume", "1s", 1, 1}},
		{[]interface{}{"alabel", 1, "invalid period", 1, 1}},
		{[]interface{}{"alabel", 1, "1s", "invalid capacity", 1}},
		{[]interface{}{"alabel", 1, "1s", 1, "invalid increment"}},
		{[]interface{}{"zero volume", 0, "1s", 1, 1}},
		{[]interface{}{"zero period", "0s", 1, 1}},
		{[]interface{}{"zero capacity", 1, "1s", 0, 1}},
		{[]interface{}{"zero increment", 1, "1s", 1, 0}},
	} {
		t.Run(fmt.Sprintf("test#%d", i), func(t *testing.T) {
			_, err := spec.CreateFilter(test.args)

			assert.Error(t, err)
		})
	}
}

func TestLeakyBucketFilterValidArgs(t *testing.T) {
	for i, test := range []struct {
		args            []interface{}
		expectCapacity  int
		expectEmission  time.Duration
		expectIncrement int
	}{
		{
			args:            []interface{}{"alabel", 4, "1s", 2, 1},
			expectCapacity:  2,
			expectEmission:  250 * time.Millisecond,
			expectIncrement: 1,
		},
		{
			args:            []interface{}{"floatargs", 4.0, "1s", 2.0, 1.0},
			expectCapacity:  2,
			expectEmission:  250 * time.Millisecond,
			expectIncrement: 1,
		},
	} {
		t.Run(fmt.Sprintf("test#%d", i), func(t *testing.T) {
			spec := &leakyBucketSpec{
				create: func(capacity int, emission time.Duration) leakyBucket {
					assert.Equal(t, test.expectCapacity, capacity)
					assert.Equal(t, test.expectEmission, emission)
					return nil
				},
			}

			f, err := spec.CreateFilter(test.args)

			assert.NoError(t, err)
			assert.Equal(t, test.expectIncrement, f.(*leakyBucketFilter).increment)
		})
	}
}

type leakyBucketFunc func(context.Context, string, int) (bool, time.Duration, error)

func (b leakyBucketFunc) Add(ctx context.Context, label string, increment int) (added bool, retry time.Duration, err error) {
	return b(ctx, label, increment)
}

func TestLeakyBucketFilterRequest(t *testing.T) {
	for _, test := range []struct {
		name       string
		args       []interface{}
		add        func(*testing.T, string, int) (bool, time.Duration, error)
		served     bool
		status     int
		retryAfter string
	}{
		{
			name: "allow on missing placeholder",
			args: []interface{}{"alabel-${missing}", 3, "1s", 2, 1},
			add: func(t *testing.T, _ string, _ int) (bool, time.Duration, error) {
				t.Error("unexpected call on missing placeholder")
				return false, 0, nil
			},
		},
		{
			name: "allow on error",
			args: []interface{}{"alabel", 3, "1s", 2, 1},
			add: func(*testing.T, string, int) (bool, time.Duration, error) {
				return false, 0, fmt.Errorf("oops")
			},
		},
		{
			name: "allow on added",
			args: []interface{}{"alabel", 3, "1s", 2, 1},
			add: func(t *testing.T, label string, increment int) (bool, time.Duration, error) {
				assert.Equal(t, "alabel", label)
				assert.Equal(t, 1, increment)
				return true, 0, nil
			},
		},
		{
			name: "allow with a placeholder",
			args: []interface{}{"alabel-${request.header.X-Foo}", 3, "1s", 2, 1},
			add: func(t *testing.T, label string, increment int) (bool, time.Duration, error) {
				assert.Equal(t, "alabel-bar", label)
				assert.Equal(t, 1, increment)
				return true, 0, nil
			},
		},
		{
			name: "deny",
			args: []interface{}{"alabel", 3, "1s", 2, 1},
			add: func(t *testing.T, label string, increment int) (bool, time.Duration, error) {
				assert.Equal(t, "alabel", label)
				assert.Equal(t, 1, increment)
				return false, 3 * time.Second, nil
			},
			served:     true,
			status:     429,
			retryAfter: "3",
		},
		{
			name: "deny with a placeholder",
			args: []interface{}{"alabel-${request.header.X-Foo}", 3, "1s", 2, 1},
			add: func(t *testing.T, label string, increment int) (bool, time.Duration, error) {
				assert.Equal(t, "alabel-bar", label)
				assert.Equal(t, 1, increment)
				return false, 3 * time.Second, nil
			},
			served:     true,
			status:     429,
			retryAfter: "3",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			spec := &leakyBucketSpec{
				create: func(capacity int, emission time.Duration) leakyBucket {
					return leakyBucketFunc(func(_ context.Context, label string, increment int) (bool, time.Duration, error) {
						return test.add(t, label, increment)
					})
				},
			}

			f, err := spec.CreateFilter(test.args)
			require.NoError(t, err)

			ctx := &filtertest.Context{
				FRequest: &http.Request{Header: http.Header{"X-Foo": []string{"bar"}}},
			}

			f.Request(ctx)

			if test.served {
				assert.True(t, ctx.FServed)
				assert.Equal(t, test.status, ctx.FResponse.StatusCode)
				assert.Equal(t, test.retryAfter, ctx.FResponse.Header.Get("Retry-After"))
			} else {
				assert.False(t, ctx.FServed)
			}
		})
	}
}
