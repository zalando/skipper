package envoy

import (
	"context"
	"strconv"
	"strings"

	"github.com/open-policy-agent/opa/v1/ast"
	"github.com/open-policy-agent/opa/v1/plugins"
	"github.com/open-policy-agent/opa/v1/util"
)

// Factory defines the interface OPA uses to instantiate a plugin.
type Factory struct{}

// New returns the object initialized with a valid plugin configuration.
func (Factory) New(m *plugins.Manager, cfg interface{}) plugins.Plugin {
	p := &Plugin{
		manager: m,
		cfg:     *cfg.(*PluginConfig),
	}

	m.UpdatePluginStatus(PluginName, &plugins.Status{State: plugins.StateNotReady})

	return p
}

// Validate returns a valid configuration to instantiate the plugin.
func (Factory) Validate(m *plugins.Manager, bs []byte) (interface{}, error) {
	cfg := PluginConfig{
		DryRun: defaultDryRun,
	}

	if err := util.Unmarshal(bs, &cfg); err != nil {
		return nil, err
	}

	if err := cfg.ParseQuery(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func (p *Plugin) Reconfigure(ctx context.Context, config interface{}) {
	p.cfg = *config.(*PluginConfig)
}

// PluginConfig represents the plugin configuration.
type PluginConfig struct {
	Path                 string `json:"path"`
	DryRun               bool   `json:"dry-run"`
	SkipRequestBodyParse bool   `json:"skip-request-body-parse"`

	ParsedQuery ast.Body
}

type Plugin struct {
	cfg     PluginConfig
	manager *plugins.Manager
}

func (p *Plugin) Start(ctx context.Context) error {
	p.manager.UpdatePluginStatus(PluginName, &plugins.Status{State: plugins.StateOK})
	return nil
}

func (cfg *PluginConfig) ParseQuery() error {
	var parsedQuery ast.Body
	var err error

	if cfg.Path == "" {
		cfg.Path = defaultPath
	}
	path := stringPathToDataRef(cfg.Path)
	parsedQuery, err = ast.ParseBody(path.String())

	if err != nil {
		return err
	}

	cfg.ParsedQuery = parsedQuery

	return nil
}

func (p *Plugin) Stop(ctx context.Context) {
	p.manager.UpdatePluginStatus(PluginName, &plugins.Status{State: plugins.StateNotReady})
}

func (p *Plugin) GetConfig() PluginConfig {
	return p.cfg
}

func (p *Plugin) ParsedQuery() ast.Body { return p.cfg.ParsedQuery }
func (p *Plugin) Path() string          { return p.cfg.Path }

func stringPathToDataRef(s string) (r ast.Ref) {
	result := ast.Ref{ast.DefaultRootDocument}
	result = append(result, stringPathToRef(s)...)
	return result
}

func stringPathToRef(s string) (r ast.Ref) {
	if len(s) == 0 {
		return r
	}

	p := strings.Split(s, "/")
	for _, x := range p {
		if x == "" {
			continue
		}

		i, err := strconv.Atoi(x)
		if err != nil {
			r = append(r, ast.StringTerm(x))
		} else {
			r = append(r, ast.IntNumberTerm(i))
		}
	}
	return r
}
