// Package eopa provides enterprise opa plugins aggregation.
package eopa

import (
	dl "github.com/open-policy-agent/eopa/pkg/plugins/decision_logs"
	"github.com/open-policy-agent/opa/v1/plugins"
)

func All() map[string]plugins.Factory {
	return map[string]plugins.Factory{
		//data.Name:       data.Factory(), // disabled as we currently do not use this plugin and due to the unexpected impact on the opa body parsing filter.
		dl.DLPluginName: dl.Factory(),
	}
}
