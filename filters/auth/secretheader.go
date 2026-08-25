package auth

import (
	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/secrets"
)

type (
	secretHeaderSpec struct {
		secretsReader secrets.SecretsReader
	}

	secretHeaderFilter struct {
		headerName string
		secretName string
		prefix     string
		suffix     string

		secretsReader secrets.SecretsReader
	}
)

func NewSetRequestHeaderFromSecret(sr secrets.SecretsReader) filters.Spec {
	return &secretHeaderSpec{secretsReader: sr}
}

func (*secretHeaderSpec) Name() string {
	return filters.SetRequestHeaderFromSecretName
}

func (s *secretHeaderSpec) CreateFilter(args []any) (filters.Filter, error) {
	if len(args) < 2 || len(args) > 4 {
		return nil, filters.ErrInvalidFilterParameters
	}
	var ok bool

	f := &secretHeaderFilter{
		secretsReader: s.secretsReader,
	}

	f.headerName, ok = args[0].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}

	f.secretName, ok = args[1].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}

	if len(args) > 2 {
		f.prefix, ok = args[2].(string)
		if !ok {
			return nil, filters.ErrInvalidFilterParameters
		}
	}

	if len(args) > 3 {
		f.suffix, ok = args[3].(string)
		if !ok {
			return nil, filters.ErrInvalidFilterParameters
		}
	}

	return f, nil
}

func (f *secretHeaderFilter) Request(ctx filters.FilterContext) {
	value, ok := f.secretsReader.GetSecret(f.secretName)
	if !ok {
		log.Errorf("Secret %q not found for setRequestHeaderFromSecret filter", f.secretName)
		return
	}
	ctx.Request().Header.Set(f.headerName, f.prefix+string(value)+f.suffix)
}

func (*secretHeaderFilter) Response(filters.FilterContext) {}
