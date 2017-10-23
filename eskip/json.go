package eskip

import (
	"bytes"
	"encoding/json"
	"errors"
)

type jsonRoute struct {
	ID         string       `json:"id"`
	Backend    string       `json:"backend"`
	Predicates []*Predicate `json:"predicates"`
	Filters    []*Filter    `json:"filters"`
}

type jsonNameArgs struct {
	Name string        `json:"name"`
	Args []interface{} `json:"args"`
}

var (
	ErrInvalidArgCount = errors.New("invalid count of args")
	ErrInvalidArgType  = errors.New("invalid arg type")
)

func marshalJSONPredicates(r *Route) []*Predicate {
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

func extractStringArgs(args []interface{}, count int) ([]string, error) {
	if count > 0 && len(args) != count {
		return nil, ErrInvalidArgCount
	}

	if count <= 0 {
		count = len(args)
	}

	sargs := make([]string, count)
	for i := range args {
		sa, ok := args[i].(string)
		if !ok {
			return nil, ErrInvalidArgType
		}

		sargs[i] = sa
	}

	return sargs, nil
}

func unmarshalJSONPredicates(r *Route, p []*Predicate) error {
	var ps []*Predicate
	for _, pi := range p {
		switch pi.Name {
		case "Method":
			a, err := extractStringArgs(pi.Args, 1)
			if err != nil {
				return err
			}

			r.Method = a[0]
		case "Path":
			a, err := extractStringArgs(pi.Args, 1)
			if err != nil {
				return err
			}

			r.Path = a[0]
		case "HostRegexp":
			a, err := extractStringArgs(pi.Args, 0)
			if err != nil {
				return err
			}

			r.HostRegexps = a
		case "PathRegexp":
			a, err := extractStringArgs(pi.Args, 0)
			if err != nil {
				return err
			}

			r.PathRegexps = a
		case "Header":
			a, err := extractStringArgs(pi.Args, 2)
			if err != nil {
				return err
			}

			if r.Headers == nil {
				r.Headers = make(map[string]string)
			}

			r.Headers[a[0]] = a[1]
		case "HeaderRegexp":
			a, err := extractStringArgs(pi.Args, 2)
			if err != nil {
				return err
			}

			if r.HeaderRegexps == nil {
				r.HeaderRegexps = make(map[string][]string)
			}

			r.HeaderRegexps[a[0]] = append(r.HeaderRegexps[a[0]], a[1])
		default:
			ps = append(ps, pi)
		}
	}

	r.Predicates = ps
	return nil
}

func marshalNameArgs(name string, args []interface{}) ([]byte, error) {
	if args == nil {
		args = []interface{}{}
	}

	return json.Marshal(&jsonNameArgs{
		Name: name,
		Args: args,
	})
}

func unmarshalNameArgs(b []byte) (string, []interface{}, error) {
	var jna jsonNameArgs
	err := json.Unmarshal(b, &jna)
	return jna.Name, jna.Args, err
}

func (f *Filter) MarshalJSON() ([]byte, error) {
	return marshalNameArgs(f.Name, f.Args)
}

func (f *Filter) UnmarshalJSON(b []byte) error {
	var err error
	f.Name, f.Args, err = unmarshalNameArgs(b)
	return err
}

func (p *Predicate) MarshalJSON() ([]byte, error) {
	return marshalNameArgs(p.Name, p.Args)
}

func (p *Predicate) UnmarshalJSON(b []byte) error {
	var err error
	p.Name, p.Args, err = unmarshalNameArgs(b)
	return err
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

	if err := e.Encode(&jsonRoute{
		ID:         r.Id,
		Backend:    backend,
		Predicates: marshalJSONPredicates(r),
		Filters:    filters,
	}); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func (r *Route) UnmarshalJSON(b []byte) error {
	var jr jsonRoute
	if err := json.Unmarshal(b, &jr); err != nil {
		return err
	}

	r.Id = jr.ID

	switch jr.Backend {
	case "<shunt>":
		r.BackendType = ShuntBackend
		r.Shunt = true
	case "<loopback>":
		r.BackendType = LoopBackend
	default:
		r.Backend = jr.Backend
	}

	if len(jr.Filters) == 0 {
		jr.Filters = nil
	}

	r.Filters = jr.Filters

	err := unmarshalJSONPredicates(r, jr.Predicates)
	if err != nil {
		return err
	}

	return nil
}
