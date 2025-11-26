// Package eopa provides enterprise opa plugins aggregation.
package eopa

import (
	"github.com/open-policy-agent/opa/v1/hooks"
	"github.com/open-policy-agent/opa/v1/logging"
	"github.com/open-policy-agent/opa/v1/plugins"
	"github.com/open-policy-agent/opa/v1/storage"

	"github.com/open-policy-agent/eopa/pkg/builtins"
	"github.com/open-policy-agent/eopa/pkg/ekm"
	eopaDl "github.com/open-policy-agent/eopa/pkg/plugins/decision_logs"
	"github.com/open-policy-agent/eopa/pkg/rego_vm"
	eopaStorage "github.com/open-policy-agent/eopa/pkg/storage"
)

func Init() (fs map[string]plugins.Factory, configHooks hooks.Hook, store storage.Store) {
	rego_vm.SetDefault(true)
	builtins.Init()

	ekmHook := ekm.NewEKM()
	ekmHook.SetLogger(logging.NewNoOpLogger())
	configHooks = hooks.New(configHooks, ekmHook)

	return Plugins(), ekmHook, eopaStorage.New()
}

func Plugins() map[string]plugins.Factory {
	return map[string]plugins.Factory{
		// data plugin is commented out as we currently do not use this and due to the unexpected impact on the opa body parsing filter.
		//data.Name:       data.Factory(),
		eopaDl.DLPluginName: eopaDl.Factory(),
	}
}
