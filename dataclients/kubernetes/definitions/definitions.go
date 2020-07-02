/* Package definitions provides type definitions, parsing, marshaling and
validation for Kubernetes resources used by Skipper. */
package definitions

import (
	"encoding/json"

	"github.com/go-yaml/yaml"
	"github.com/pkg/errors"
)

type RouteGroupList struct {
	Items []*RouteGroupItem `json:"items"`
}

type RouteGroupItem struct {
	Metadata *metadata       `json:"metadata"`
	spec     *routeGroupSpec `json:"spec"`
}

type IngressList struct {
	Items []*IngressItem `json:"items"`
}

type IngressItem struct {
	Metadata *metadata    `json:"metadata"`
	Spec     *ingressSpec `json:"spec"`
}

func (rl RouteGroupList) MarshalJSON() ([]byte, error) {
	b, err := json.Marshal(rl)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func (rl RouteGroupList) UnmarshalJSON(d []byte) error {
	return json.Unmarshal(d, &rl)
}

func (rl RouteGroupList) MarshalYAML() ([]byte, error) {
	b, err := yaml.Marshal(rl)
	if err != nil {
		return nil, err
	}
	return b, nil
}

// TODO: figure out if this should implement github.com/go-yaml/yaml/yaml.go
//  Marshaler interface
// UnmarshalYAML unmarshalls RouteGroupList
func (rl RouteGroupList) UnmarshalYAML(d []byte) error {
	return yaml.Unmarshal(d, &rl)
}

func (r RouteGroupItem) MarshalJSON() ([]byte, error) {
	b, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func (r RouteGroupItem) UnmarshalJSON(d []byte) error {
	return json.Unmarshal(d, &r)
}

// MarshalYAML marshals RouteGroupItem
func (r RouteGroupItem) MarshalYAML() ([]byte, error) {
	b, err := yaml.Marshal(r)
	if err != nil {
		return nil, err
	}
	return b, nil
}

// UnmarshalYAML unmarshals RouteGroupItem
func (r RouteGroupItem) UnmarshalYAML(d []byte) error {
	return yaml.Unmarshal(d, &r)
}

// ParseRouteGroupsJSON parses a json list of Rouetegroups into RouteGroupList
func ParseRouteGroupsJSON(d []byte) (RouteGroupList, error) {
	var rl RouteGroupList
	err := rl.UnmarshalJSON(d)
	return rl, err
}

// ParseRouteGroupsYAML parses a YAML list of Rouetegroups into RouteGroupList
func ParseRouteGroupsYAML(d []byte) (RouteGroupList, error) {
	var rl RouteGroupList
	// TODO: implement
	err := rl.UnmarshalYAML(d)
	return rl, err
}

// ValidateRouteGroup validates RouteGroupItem
func ValidateRouteGroup(rg *RouteGroupItem) error {
	return rg.validate()
}

// ValidateRouteGroups validates RouteGroupList
func ValidateRouteGroups(rl RouteGroupList) error {
	var err error
	// avoid the user having to repeatedly validate to discover all errors
	for _, i := range rl.Items {
		nerr := ValidateRouteGroup(i)
		if nerr != nil {
			err = errors.Wrap(err, nerr.Error())
		}
	}

	if err != nil {
		return err
	}

	return nil
}

func (IngressList) MarshalJSON() ([]byte, error)   {}
func (IngressList) UnmarshalJSON() ([]byte, error) {}
func (IngressList) MarshalYAML() ([]byte, error)   {}
func (IngressList) UnmarshalYAML() ([]byte, error) {}
func (IngressItem) MarshalJSON() ([]byte, error)   {}
func (IngressItem) UnmarshalJSON() ([]byte, error) {}
func (IngressItem) MarshalYAML() ([]byte, error)   {}
func (IngressItem) UnmarshalYAML() ([]byte, error) {}
func ParseIngressJSON([]byte) (IngressList, error) {}
func ParseIngressYAML([]byte) (IngressList, error) {}

func ValidateIngress(_ *IngressItem) error {
	return nil
}

func ValidateIngresses(ingressList IngressList) error {
	var err error
	// avoid the user having to repeatedly validate to discover all errors
	for _, i := range ingressList.Items {
		nerr := ValidateIngress(i)
		if nerr != nil {
			err = errors.Wrap(err, nerr.Error())
		}
	}

	if err != nil {
		return err
	}

	return nil
}
