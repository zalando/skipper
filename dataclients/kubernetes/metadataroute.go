package kubernetes

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/predicates"
	"github.com/zalando/skipper/routing"

	snet "github.com/zalando/skipper/net"
)

type MetadataPreProcessorOptions struct {
	EndpointRegistry *routing.EndpointRegistry
}

type metadataPreProcessor struct {
	options MetadataPreProcessorOptions
}

type kubeRouteMetadata struct {
	Addresses map[string]*kubeRouteMetadataAddress `json:"addresses"`
}

type kubeRouteMetadataAddress struct {
	Zone     string `json:"zone,omitempty"`
	NodeName string `json:"nodeName,omitempty"`
	PodName  string `json:"podName,omitempty"`
}

// NewMetadataPreProcessor creates pre-processor for metadata route.
func NewMetadataPreProcessor(options MetadataPreProcessorOptions) routing.PreProcessor {
	return &metadataPreProcessor{options: options}
}

func (pp *metadataPreProcessor) Do(routes []*eskip.Route) []*eskip.Route {
	var metadataRoute *eskip.Route
	filtered := make([]*eskip.Route, 0, len(routes))

	for _, r := range routes {
		if r.Id == MetadataRouteID {
			if metadataRoute == nil {
				metadataRoute = r
			} else {
				log.Errorf("Found multiple metadata routes, using the first one")
			}
		} else {
			filtered = append(filtered, r)
		}
	}

	if metadataRoute == nil {
		log.Errorf("Metadata route not found")
		return routes
	}

	metadata, err := decodeMetadata(metadataRoute)
	if err != nil {
		log.Errorf("Failed to decode metadata route: %v", err)
		return filtered
	}

	for _, r := range filtered {
		if r.BackendType == eskip.NetworkBackend {
			pp.addMetadata(metadata, r.Backend)
		} else if r.BackendType == eskip.LBBackend {
			for _, ep := range r.LBEndpoints {
				pp.addMetadata(metadata, ep)
			}
		}
	}
	return filtered
}

// metadataRoute creates a route with [MetadataRouteID] id that matches no requests and
// contains metadata for each endpoint address used by Ingresses and RouteGroups.
func metadataRoute(s *clusterState) *eskip.Route {
	metadata := kubeRouteMetadata{
		Addresses: make(map[string]*kubeRouteMetadataAddress),
	}

	for id := range s.cachedEndpoints {
		if s.enableEndpointSlices {
			if eps, ok := s.endpointSlices[id.ResourceID]; ok {
				for _, ep := range eps.Endpoints {
					metadata.Addresses[ep.Address] = &kubeRouteMetadataAddress{
						Zone:     ep.Zone,
						NodeName: ep.NodeName,
						PodName:  ep.TargetRef.getPodName(),
					}
				}
			}
		} else {
			if ep, ok := s.endpoints[id.ResourceID]; ok {
				for _, subset := range ep.Subsets {
					for _, addr := range subset.Addresses {
						metadata.Addresses[addr.IP] = &kubeRouteMetadataAddress{
							// Endpoints do not provide zone
							NodeName: addr.NodeName,
							PodName:  addr.TargetRef.getPodName(),
						}
					}
				}
			}
		}
	}

	return &eskip.Route{
		Id:          MetadataRouteID,
		Predicates:  []*eskip.Predicate{{Name: predicates.FalseName}},
		BackendType: eskip.NetworkBackend,
		Backend:     encodeDataURI(&metadata),
	}
}

func decodeMetadata(r *eskip.Route) (map[string]*kubeRouteMetadataAddress, error) {
	metadata, err := decodeDataURI(r.Backend)
	if err != nil {
		return nil, err
	}
	return metadata.Addresses, nil
}

const dataUriPrefix = "data:application/json;base64,"

// encodeDataURI encodes metadata into data URI.
// Note that map keys are sorted and used as JSON object keys
// therefore encodeDataURI produces the same output for the same input.
// See https://datatracker.ietf.org/doc/html/rfc2397
func encodeDataURI(metadata *kubeRouteMetadata) string {
	data, _ := json.Marshal(&metadata)

	buf := make([]byte, len(dataUriPrefix)+base64.StdEncoding.EncodedLen(len(data)))

	copy(buf, dataUriPrefix)
	base64.StdEncoding.Encode(buf[len(dataUriPrefix):], data)

	return string(buf)
}

// encodeDataURI encodes metadata into data URI.
// See https://datatracker.ietf.org/doc/html/rfc2397
func decodeDataURI(uri string) (*kubeRouteMetadata, error) {
	var metadata kubeRouteMetadata

	data, err := base64.StdEncoding.DecodeString(uri[len(dataUriPrefix):])
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64: %w", err)
	}

	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, fmt.Errorf("failed to decode json: %w", err)
	}
	return &metadata, nil
}

func (pp *metadataPreProcessor) addMetadata(metadata map[string]*kubeRouteMetadataAddress, endpoint string) {
	_, hostPort, err := snet.SchemeHost(endpoint)
	if err != nil {
		return
	}

	host, _, _ := net.SplitHostPort(hostPort)
	if err != nil {
		host = hostPort
	}

	addr, ok := metadata[host]
	if !ok {
		return
	}

	metrics := pp.options.EndpointRegistry.GetMetrics(hostPort)
	setTag := func(name, value string) {
		if value != "" {
			metrics.SetTag(name, value)
		}
	}

	setTag("zone", addr.Zone)
	setTag("nodeName", addr.NodeName)
	setTag("podName", addr.PodName)
}
