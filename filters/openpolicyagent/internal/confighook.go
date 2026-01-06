// Package internal provides internal only code to be able to use quasi
// standard OPA plugins and config.
package internal

import (
	"context"
	"encoding/json"

	"github.com/open-policy-agent/opa/v1/config"
	"github.com/open-policy-agent/opa/v1/plugins"
	"github.com/open-policy-agent/opa/v1/plugins/bundle"
	"github.com/open-policy-agent/opa/v1/plugins/discovery"
)

// ManualOverride is override the plugin trigger to manual trigger mode, allowing the openpolicyagent filter
// to take control of the trigger mechanism.
//   - OnConfig will handle a general config change and will override the trigger mode for both discovery
//     and bundle plugins.
//   - OnConfigDiscovery will handle a config change via discovery mechanism and will only override
//     the trigger mode for the bundle plugin as the discovery plugin is involved in the trigger for
//     this config change.
//
// See https://github.com/open-policy-agent/opa/pull/6053 for details on the hooks.
type ManualOverride struct {
}

func (m *ManualOverride) OnConfig(ctx context.Context, config *config.Config) (*config.Config, error) {
	config, err := discoveryPluginOverride(config)
	if err != nil {
		return nil, err
	}
	return bundlePluginConfigOverride(config)
}

func (m *ManualOverride) OnConfigDiscovery(ctx context.Context, config *config.Config) (*config.Config, error) {
	return bundlePluginConfigOverride(config)
}

func discoveryPluginOverride(config *config.Config) (*config.Config, error) {
	var (
		discoveryConfig discovery.Config
		triggerManual   = plugins.TriggerManual
		message         []byte
	)

	if config.Discovery != nil {
		err := json.Unmarshal(config.Discovery, &discoveryConfig)
		if err != nil {
			return nil, err
		}
		discoveryConfig.Trigger = &triggerManual

		message, err = json.Marshal(discoveryConfig)
		if err != nil {
			return nil, err
		}
		config.Discovery = message
	}
	return config, nil
}

func bundlePluginConfigOverride(config *config.Config) (*config.Config, error) {
	var (
		bundlesConfig map[string]*bundle.Source
		manualTrigger = plugins.TriggerManual
		message       []byte
	)

	if config.Bundles != nil {
		err := json.Unmarshal(config.Bundles, &bundlesConfig)
		if err != nil {
			return nil, err
		}
		for _, bndlCfg := range bundlesConfig {
			bndlCfg.Trigger = &manualTrigger
		}
		message, err = json.Marshal(bundlesConfig)
		if err != nil {
			return nil, err
		}
		config.Bundles = message
	}
	return config, nil
}
