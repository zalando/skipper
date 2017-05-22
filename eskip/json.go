package eskip

import (
	"bytes"
	"encoding/json"
)

func marshalJsonPredicates(r *Route) []*Predicate {
	rjf := make([]*Predicate, 0, len(r.Predicates))

	if r.Method != "" {
		rjf = append(rjf, &Predicate{
			Name: "Method",
			Args: []interface{}{r.Method},
		})
	}

	if r.Path != "" {
		rjf = append(rjf, &Predicate{
			Name: "Path",
			Args: []interface{}{r.Path},
		})
	}

	for _, h := range r.HostRegexps {
		rjf = append(rjf, &Predicate{
			Name: "HostRegexp",
			Args: []interface{}{h},
		})
	}

	for _, p := range r.PathRegexps {
		rjf = append(rjf, &Predicate{
			Name: "PathRegexp",
			Args: []interface{}{p},
		})
	}

	for k, v := range r.Headers {
		rjf = append(rjf, &Predicate{
			Name: "Header",
			Args: []interface{}{k, v},
		})
	}

	for k, list := range r.HeaderRegexps {
		for _, v := range list {
			rjf = append(rjf, &Predicate{
				Name: "HeaderRegexp",
				Args: []interface{}{k, v},
			})
		}
	}

	rjf = append(rjf, r.Predicates...)

	return rjf
}

func marshalNameArgs(name string, args []interface{}) ([]byte, error) {
	if args == nil {
		args = []interface{}{}
	}

	return json.Marshal(&struct {
		Name string        `json:"name"`
		Args []interface{} `json:"args"`
	}{
		Name: name,
		Args: args,
	})
}

func (f *Filter) MarshalJSON() ([]byte, error) {
	return marshalNameArgs(f.Name, f.Args)
}

func (p *Predicate) MarshalJSON() ([]byte, error) {
	return marshalNameArgs(p.Name, p.Args)
}

func (r *Route) MarshalJSON() ([]byte, error) {
	backend := r.backendString()

	filters := r.Filters
	if filters == nil {
		filters = []*Filter{}
	}

	var buf bytes.Buffer
	e := json.NewEncoder(&buf)
	e.SetEscapeHTML(false)

	if err := e.Encode(&struct {
		Id         string       `json:"id"`
		Backend    string       `json:"backend"`
		Predicates []*Predicate `json:"predicates"`
		Filters    []*Filter    `json:"filters"`
	}{
		Id:         r.Id,
		Backend:    backend,
		Predicates: marshalJsonPredicates(r),
		Filters:    filters,
	}); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
