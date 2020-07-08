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
	Metadata *Metadata       `json:"metadata"`
	Spec     *RouteGroupSpec `json:"spec"`
}

type IngressList struct {
	Items []*IngressItem `json:"items"`
}

type IngressItem struct {
	Metadata *Metadata    `json:"metadata"`
	Spec     *ingressSpec `json:"spec"`
}

// MarshalJSON marshals a RouteGroupList
func (rl *RouteGroupList) MarshalJSON() ([]byte, error) {
	b, err := json.Marshal(rl)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func (rl *RouteGroupList) UnmarshalJSON(d []byte) error {
	return json.Unmarshal(d, rl)
}

// MarshalYAML marshals a RouteGroupList
func (rl *RouteGroupList) MarshalYAML() ([]byte, error) {
	b, err := yaml.Marshal(rl)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func (rl *RouteGroupList) UnmarshalYAML(d []byte) error {
	return yaml.Unmarshal(d, rl)
}

// MarshalJSON marshals RouteGroupItem
func (r *RouteGroupItem) MarshalJSON() ([]byte, error) {
	b, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func (r *RouteGroupItem) UnmarshalJSON(d []byte) error {
	return json.Unmarshal(d, r)
}

// MarshalYAML marshals RouteGroupItem
func (r *RouteGroupItem) MarshalYAML() ([]byte, error) {
	b, err := yaml.Marshal(r)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func (r *RouteGroupItem) UnmarshalYAML(d []byte) error {
	return yaml.Unmarshal(d, &r)
}

// ParseRouteGroupsJSON parses a json list of RouteGroups into RouteGroupList
func ParseRouteGroupsJSON(d []byte) (RouteGroupList, error) {
	var rl RouteGroupList
	err := rl.UnmarshalJSON(d)
	return rl, err
}

// ParseRouteGroupsYAML parses a YAML list of RouteGroups into RouteGroupList
func ParseRouteGroupsYAML(d []byte) (RouteGroupList, error) {
	var rl RouteGroupList
	err := rl.UnmarshalYAML(d)
	return rl, err
}

// ValidateRouteGroup validates a RouteGroupItem
func ValidateRouteGroup(rg *RouteGroupItem) error {
	return rg.validate()
}

// ValidateRouteGroups validates a RouteGroupList
func ValidateRouteGroups(rl *RouteGroupList) error {
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

// MarshalJSON marshals an IngressList
func (il *IngressList) MarshalJSON() ([]byte, error) {
	return json.Marshal(il)
}

func (il *IngressList) UnmarshalJSON(d []byte) error {
	return json.Unmarshal(d, il)
}

// MarshalYAML marshals an IngressList
func (il *IngressList) MarshalYAML() ([]byte, error) {
	return yaml.Marshal(il)
}

func (il *IngressList) UnmarshalYAML(d []byte) error {
	return yaml.Unmarshal(d, il)
}

// MarshalJSON marshals an IngressItem
func (i *IngressItem) MarshalJSON() ([]byte, error) {
	return json.Marshal(i)
}

func (i *IngressItem) UnmarshalJSON(d []byte) error {
	return json.Unmarshal(d, i)
}

// MarshalYAML marshals an IngressItem
func (i *IngressItem) MarshalYAML() ([]byte, error) {
	return yaml.Marshal(i)
}
func (i *IngressItem) UnmarshalYAML(d []byte) error {
	return yaml.Unmarshal(d, i)
}

// ParseIngressJSON parse JSON into an IngressList
func ParseIngressJSON(d []byte) (IngressList, error) {
	var il IngressList
	err := il.UnmarshalJSON(d)
	return il, err
}

// ParseIngressYAML parse YAML into an IngressList
func ParseIngressYAML(d []byte) (IngressList, error) {
	var il IngressList
	err := il.UnmarshalYAML(d)
	return il, err
}

// TODO: implement
// ValidateIngress is a no-op
func ValidateIngress(_ *IngressItem) error {
	return nil
}

// ValidateIngresses is a no-op
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
