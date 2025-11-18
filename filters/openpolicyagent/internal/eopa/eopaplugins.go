package eopa

import (
	"github.com/open-policy-agent/eopa/pkg/plugins/data"
	dl "github.com/open-policy-agent/eopa/pkg/plugins/decision_logs"
	"github.com/open-policy-agent/opa/v1/plugins"
)

func All() map[string]plugins.Factory {
	return map[string]plugins.Factory{
		data.Name:       data.Factory(),
		dl.DLPluginName: dl.Factory(),
	}
}
