/* Package definitions provides type definitions, parsing, marshaling and
validation for Kubernetes resources used by Skipper. */
package definitions

import (
	"encoding/json"
	"fmt"

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
	Spec     *IngressSpec `json:"spec"`
}

// ParseRouteGroupsJSON parses a json list of RouteGroups into RouteGroupList
func ParseRouteGroupsJSON(d []byte) (RouteGroupList, error) {
	var rl RouteGroupList
	err := json.Unmarshal(d, &rl)
	return rl, err
}

// ParseRouteGroupsYAML parses a YAML list of RouteGroups into RouteGroupList
func ParseRouteGroupsYAML(d []byte) (RouteGroupList, error) {
	var rl RouteGroupList
	err := yaml.Unmarshal(d, &rl)
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

// ParseIngressJSON parse JSON into an IngressList
func ParseIngressJSON(d []byte) (IngressList, error) {
	var il IngressList
	err := json.Unmarshal(d, &il)
	return il, err
}

// ParseIngressYAML parse YAML into an IngressList
func ParseIngressYAML(d []byte) (IngressList, error) {
	var il IngressList
	err := yaml.Unmarshal(d, &il)
	return il, err
}

// TODO: implement once IngressItem has a validate method
// ValidateIngress is a no-op
func ValidateIngress(_ *IngressItem) error {
	return nil
}

// ValidateIngresses is a no-op
func ValidateIngresses(ingressList IngressList) error {
	var err error
	// discover all errors to avoid the user having to repeatedly validate
	for _, i := range ingressList.Items {
		nerr := ValidateIngress(i)
		if nerr != nil {
			name := i.Metadata.Name
			namespace := i.Metadata.Namespace
			nerr = fmt.Errorf("%s/%s: %w", name, namespace, nerr)
			err = errors.Wrap(err, nerr.Error())
		}
	}

	if err != nil {
		return err
	}

	return nil
}
