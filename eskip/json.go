package eskip

import (
	"bytes"
	"encoding/json"
)

type jsonNameArgs struct {
	Name string        `json:"name"`
	Args []interface{} `json:"args,omitempty"`
}

type jsonBackend struct {
	Type      string   `json:"type"`
	Address   string   `json:"address,omitempty"`
	Algorithm string   `json:"algorithm,omitempty"`
	Endpoints []string `json:"endpoints,omitempty"`
}

type jsonRoute struct {
	ID         string       `json:"id,omitempty"`
	Backend    *jsonBackend `json:"backend,omitempty"`
	Predicates []*Predicate `json:"predicates,omitempty"`
	Filters    []*Filter    `json:"filters,omitempty"`
}

func newJSONRoute(r *Route) *jsonRoute {
	cr := Canonical(r)
	jr := &jsonRoute{
		ID:         cr.Id,
		Predicates: cr.Predicates,
		Filters:    cr.Filters,
	}

	if cr.BackendType != NetworkBackend || cr.Backend != "" {
		jr.Backend = &jsonBackend{
			Type:      cr.BackendType.String(),
			Address:   cr.Backend,
			Algorithm: cr.LBAlgorithm,
			Endpoints: LBEndpointString(cr.LBEndpoints),
		}
	}

	return jr
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
	return marshalJSONNoEscape(&jsonNameArgs{Name: f.Name, Args: f.Args})
}

func (p *Predicate) MarshalJSON() ([]byte, error) {
	return marshalJSONNoEscape(&jsonNameArgs{Name: p.Name, Args: p.Args})
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

	var bts string
	if jr.Backend != nil {
		bts = jr.Backend.Type
	}

	bt, err := BackendTypeFromString(bts)
	if err != nil {
		return err
	}

	r.BackendType = bt
	switch bt {
	case NetworkBackend:
		if jr.Backend != nil {
			r.Backend = jr.Backend.Address
		}
	case LBBackend:
		r.LBAlgorithm = jr.Backend.Algorithm
		r.LBEndpoints = NewLBEndpoints(jr.Backend.Endpoints)
		if len(r.LBEndpoints) == 0 {
			r.LBEndpoints = nil
		}
	}

	r.Filters = jr.Filters
	if len(r.Filters) == 0 {
		r.Filters = nil
	}

	r.Predicates = jr.Predicates
	if len(r.Predicates) == 0 {
		r.Predicates = nil
	}

	return nil
}
