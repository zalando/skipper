package auth

import (
	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/secrets"
)

const (
	// Deprecated, use filters.BearerInjectorName instead
	BearerInjectorName = filters.BearerInjectorName
)

type (
	bearerInjectorSpec struct {
		secretsReader secrets.SecretsReader
	}
	bearerInjectorFilter struct {
		secretName    string
		secretsReader secrets.SecretsReader
	}
)

func NewBearerInjector(sr secrets.SecretsReader) filters.Spec {
	return &bearerInjectorSpec{
		secretsReader: sr,
	}
}

func (*bearerInjectorSpec) Name() string {
	return filters.BearerInjectorName
}

func (b *bearerInjectorSpec) CreateFilter(args []any) (filters.Filter, error) {
	if len(args) != 1 {
		return nil, filters.ErrInvalidFilterParameters
	}
	secretName, ok := args[0].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}

	return newBearerInjectorFilter(secretName, b.secretsReader), nil
}

func newBearerInjectorFilter(s string, sr secrets.SecretsReader) *bearerInjectorFilter {
	return &bearerInjectorFilter{
		secretName:    s,
		secretsReader: sr,
	}
}

func (f *bearerInjectorFilter) Request(ctx filters.FilterContext) {
	b, ok := f.secretsReader.GetSecret(f.secretName)
	if !ok {
		log.Errorf("Secret %q not found for bearerinjector filter", f.secretName)
		return
	}
	ctx.Request().Header.Set(authHeaderName, authHeaderPrefix+string(b))
}

func (*bearerInjectorFilter) Response(filters.FilterContext) {}
