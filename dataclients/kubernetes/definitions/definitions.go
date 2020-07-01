/* Package definitions provides type definitions, parsing, marshaling and
validation for Kubernetes resources used by Skipper. */
package definitions

import (
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

func (RouteGroupList) MarshalJSON() ([]byte, error)      {}
func (RouteGroupList) UnmarshalJSON() ([]byte, error)    {}
func (RouteGroupList) MarshalYAML() ([]byte, error)      {}
func (RouteGroupList) UnmarshalYAML() ([]byte, error)    {}
func (RouteGroupItem) MarshalJSON() ([]byte, error)      {}
func (RouteGroupItem) UnmarshalJSON() ([]byte, error)    {}
func (RouteGroupItem) MarshalYAML() ([]byte, error)      {}
func (RouteGroupItem) UnmarshalYAML() ([]byte, error)    {}
func ParseRouteGroupJSON([]byte) (RouteGroupList, error) {}
func ParseRouteGroupYAML([]byte) (RouteGroupList, error) {}

// ValidateRouteGroup validates RouteGroupItem
func ValidateRouteGroup(rg *RouteGroupItem) error {
	return rg.validate()
}

// ValidateRouteGroups validates RouteGroupList
func ValidateRouteGroups(rgs RouteGroupList) error {
	var err error
	// avoid the user having to repeatedly validate to discover all errors
	for _, i := range rgs.Items {
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
