package routing

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/zalando/skipper/metrics/metricstest"
)

func TestValidationErrors(t *testing.T) {
	for _, tt := range []struct {
		name    string
		err     error
		wantErr error
	}{
		{
			name:    "test UnknownFilter",
			err:     errUnknownFilter,
			wantErr: errUnknownFilter,
		},
		{
			name:    "test InvalidFilterParams",
			err:     errInvalidFilterParams,
			wantErr: errInvalidFilterParams,
		},
		{
			name:    "test UnknownPredicate",
			err:     errUnknownPredicate,
			wantErr: errUnknownPredicate,
		},
		{
			name:    "test InvalidPredicateParams",
			err:     errInvalidPredicateParams,
			wantErr: errInvalidPredicateParams,
		},
		{
			name:    "test FailedBackendSplit",
			err:     errFailedBackendSplit,
			wantErr: errFailedBackendSplit,
		},
		{
			name:    "test InvalidMatcher",
			err:     errInvalidMatcher,
			wantErr: errInvalidMatcher,
		}} {
		t.Run(tt.name, func(t *testing.T) {
			mtr := &metricstest.MockMetrics{}
			err := HandleValidationError(mtr, tt.err, "r")
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Failed to get error %T is not %T", err, tt.wantErr)
			}

			var (
				mKey string
				mVal float64
			)
			mtr.WithGauges(func(g map[string]float64) {
				for k, v := range g {
					if strings.HasPrefix(k, "route.invalid") {
						mKey = k
						mVal = v
					}
				}
			})

			if mKey == "" {
				t.Fatal("Failed to get metric key with prefix \"route.invalid\"")
			}
			if mVal != 1 {
				t.Fatalf("Faile to get metric value 1 for key %q, got %0.2f", mKey, mVal)
			}

		})
	}

}

func TestValidationErrorsNoError(t *testing.T) {
	routeID := "r"
	mtr := &metricstest.MockMetrics{}
	mtr.SetInvalidRoute(routeID, errInvalidPredicateParams.Error())

	err := HandleValidationError(mtr, nil, routeID)
	if err != nil {
		t.Fatalf("Failed to get no error, got: %v", err)
	}

	mtr.WithGauges(func(g map[string]float64) {
		key := fmt.Sprintf("route.invalid.%s..%s", routeID, errInvalidPredicateParams)
		if v := g[key]; v != 1 {
			t.Fatalf("Invalid route metric should remain set, got %0.2f", v)
		}
	})
}

func TestWrapInvalidDefinitionReason(t *testing.T) {
	for _, tt := range []struct {
		name    string
		err     error
		wantErr error
	}{
		{
			name:    "test nil is not an error",
			err:     nil,
			wantErr: nil,
		},
		{
			name:    "test InvalidMatcher is wrapped",
			err:     errInvalidMatcher,
			wantErr: errInvalidMatcher,
		}} {
		t.Run(tt.name, func(t *testing.T) {
			err := WrapInvalidDefinitionReason("reason", tt.err)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Failed to wrap error, want %v, got %v", err, tt.wantErr)
			}
		})
	}
}
