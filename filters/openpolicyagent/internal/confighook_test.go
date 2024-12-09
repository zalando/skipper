package internal

import (
	"context"
	"encoding/json"
	"github.com/open-policy-agent/opa/v1/config"
	"github.com/open-policy-agent/opa/v1/plugins"
	"github.com/open-policy-agent/opa/v1/plugins/bundle"
	"github.com/open-policy-agent/opa/v1/plugins/discovery"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestManualOverride_OnConfig_WithDiscoveryConfig(t *testing.T) {
	config := config.Config{
		Discovery: []byte(`{
			"resource": "discovery",
			"service": "test",
			"persist": false
		}`),
	}

	m := &ManualOverride{}
	m.OnConfig(context.Background(), &config)

	var discoveryConfig discovery.Config

	err := json.Unmarshal(config.Discovery, &discoveryConfig)
	assert.NoError(t, err)

	// verify trigger is set to manual
	assert.Equal(t, *discoveryConfig.Trigger, plugins.TriggerManual)

	// verify existing config is untouched
	assert.Equal(t, *discoveryConfig.Resource, "discovery")
	assert.Equal(t, discoveryConfig.Service, "test")
	assert.Equal(t, discoveryConfig.Persist, false)
}

func TestManualOverride_OnConfig_WithBundlesConfig(t *testing.T) {
	config := config.Config{
		Bundles: []byte(`{"test":{
			"resource": "test-bundle",
			"service": "test",
			"persist": false
		}}`),
	}

	m := &ManualOverride{}
	m.OnConfig(context.Background(), &config)

	var bundlesConfig map[string]*bundle.Source

	err := json.Unmarshal(config.Bundles, &bundlesConfig)
	assert.NoError(t, err)

	// verify trigger is set to manual
	assert.Equal(t, *bundlesConfig["test"].Trigger, plugins.TriggerManual)
}

func TestManualOverride_OnConfig_WithEmptyConfig(t *testing.T) {
	config := &config.Config{}

	m := &ManualOverride{}
	processedConfig, err := m.OnConfig(context.Background(), config)

	assert.NoError(t, err)
	assert.Equal(t, config, processedConfig)
}

func TestManualOverride_OnConfig_InvalidDiscoveryConfig(t *testing.T) {
	config := config.Config{
		Discovery: []byte(`invalid`),
	}

	m := &ManualOverride{}
	_, err := m.OnConfig(context.Background(), &config)

	assert.ErrorContains(t, err, "invalid character")
}

func TestManualOverride_OnConfig_InvalidBundlesConfig(t *testing.T) {
	config := config.Config{
		Bundles: []byte(`invalid`),
	}

	m := &ManualOverride{}
	_, err := m.OnConfig(context.Background(), &config)

	assert.ErrorContains(t, err, "invalid character")
}

func TestManualOverride_OnConfigDiscovery(t *testing.T) {
	config := config.Config{
		Bundles: []byte(`{"test":{
			"resource": "test-bundle",
			"service": "test",
			"persist": false
		}}`),
	}

	m := &ManualOverride{}
	m.OnConfigDiscovery(context.Background(), &config)

	var bundlesConfig map[string]*bundle.Source

	err := json.Unmarshal(config.Bundles, &bundlesConfig)
	assert.NoError(t, err)

	// verify trigger is set to manual
	assert.Equal(t, *bundlesConfig["test"].Trigger, plugins.TriggerManual)

	// verify existing config is untouched
	assert.Equal(t, bundlesConfig["test"].Resource, "test-bundle")
	assert.Equal(t, bundlesConfig["test"].Service, "test")
	assert.Equal(t, bundlesConfig["test"].Persist, false)
}

func TestManualOverride_OnConfigDiscovery_WithoutBundlesConfig(t *testing.T) {
	config := &config.Config{}

	m := &ManualOverride{}
	processedConfig, err := m.OnConfigDiscovery(context.Background(), config)

	assert.NoError(t, err)
	assert.Equal(t, config, processedConfig)
}

func TestManualOverride_OnConfigDiscovery_InvalidBundlesConfig(t *testing.T) {
	config := config.Config{
		Bundles: []byte(`invalid`),
	}

	m := &ManualOverride{}
	_, err := m.OnConfigDiscovery(context.Background(), &config)

	assert.ErrorContains(t, err, "invalid character")
}
