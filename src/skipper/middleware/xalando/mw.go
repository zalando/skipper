package xalando

import "skipper/middleware/noop"

const name = "xalando"

type impl struct {
	noop.Type
}

func Make() *impl {
	return &impl{}
}

func (mw *impl) Name() string {
	return name
}
