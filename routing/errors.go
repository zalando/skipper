package routing

import (
	"errors"
	"fmt"

	"github.com/zalando/skipper/metrics"
)

type invalidDefinitionError string

func (e invalidDefinitionError) Error() string { return string(e) }
func (e invalidDefinitionError) Code() string  { return string(e) }

var (
	errUnknownFilter          = invalidDefinitionError("unknown_filter")
	errInvalidFilterParams    = invalidDefinitionError("invalid_filter_params")
	errUnknownPredicate       = invalidDefinitionError("unknown_predicate")
	errInvalidPredicateParams = invalidDefinitionError("invalid_predicate_params")
	errFailedBackendSplit     = invalidDefinitionError("failed_backend_split")
	errInvalidMatcher         = invalidDefinitionError("invalid_matcher")
)

func WrapInvalidDefinitionReason(reason string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %w", invalidDefinitionError(reason), err)
}

func HandleValidationError(mtr metrics.Metrics, err error, routeId string) error {
	if err == nil {
		return nil
	}

	var defErr invalidDefinitionError
	reason := "other"
	if errors.As(err, &defErr) {
		reason = defErr.Code()
	}
	mtr.SetInvalidRoute(routeId, reason)

	return fmt.Errorf("%s: %w", reason, err)
}
