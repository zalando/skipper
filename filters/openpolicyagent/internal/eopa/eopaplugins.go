// Package eopa provides enterprise opa plugins aggregation.
package eopa

import (
	dl "github.com/open-policy-agent/eopa/pkg/plugins/decision_logs"
	"github.com/open-policy-agent/opa/v1/plugins"
)

func All() map[string]plugins.Factory {
	return map[string]plugins.Factory{
		// data plugin is commented out as we currently do not use this and due to the unexpected impact on the opa body parsing filter.
		//data.Name:       data.Factory(),
		dl.DLPluginName: dl.Factory(),
	}
}
