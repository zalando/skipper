package eskip

import (
	"bytes"
	"encoding/json"
)

type jsonNameArgs struct {
	Name string        `json:"name"`
	Args []interface{} `json:"args"`
}

func newJSONNameArgs(name string, args []interface{}) *jsonNameArgs {
	if args == nil {
		args = []interface{}{}
	}

	return &jsonNameArgs{Name: name, Args: args}
}

type jsonBackend struct {
	Type      string   `json:"type"`
	Address   string   `json:"address,omitempty"`
	Algorithm string   `json:"algorithm,omitempty"`
	Endpoints []string `json:"endpoints,omitempty"`
}

type jsonRoute struct {
	ID         string       `json:"id"`
	Backend    *jsonBackend `json:"backend"`
	Predicates []*Predicate `json:"predicates"`
	Filters    []*Filter    `json:"filters"`
}

func newJSONRoute(r *Route) *jsonRoute {
	cr := Canonical(r)

	return &jsonRoute{
		ID: cr.Id,
		Backend: &jsonBackend{
			Type:      cr.BackendType.String(),
			Address:   cr.Backend,
			Algorithm: cr.LBAlgorithm,
			Endpoints: cr.LBEndpoints,
		},
		Predicates: cr.Predicates,
		Filters:    cr.Filters,
	}
}

func marshalJSONNoEscape(v interface{}) ([]byte, error) {
	var buf bytes.Buffer
	e := json.NewEncoder(&buf)
	e.SetEscapeHTML(false)

	if err := e.Encode(v); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func (f *Filter) MarshalJSON() ([]byte, error) {
	return marshalJSONNoEscape(newJSONNameArgs(f.Name, f.Args))
}

func (p *Predicate) MarshalJSON() ([]byte, error) {
	return marshalJSONNoEscape(newJSONNameArgs(p.Name, p.Args))
}

func (r *Route) MarshalJSON() ([]byte, error) {
	return marshalJSONNoEscape(newJSONRoute(r))
}

func (r *Route) UnmarshalJSON(b []byte) error {
	jr := &jsonRoute{}
	if err := json.Unmarshal(b, jr); err != nil {
		return err
	}

	r.Id = jr.ID

	bt, err := BackendTypeFromString(jr.Backend.Type)
	if err != nil {
		return err
	}
	r.BackendType = bt
	switch bt {
	case NetworkBackend:
		r.Backend = jr.Backend.Address
	case LBBackend:
		r.LBAlgorithm = jr.Backend.Algorithm
		r.LBEndpoints = jr.Backend.Endpoints
	}

	r.Filters = jr.Filters
	r.Predicates = jr.Predicates

	return nil
}
