package kubernetestest

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	yaml2 "github.com/ghodss/yaml"
	"gopkg.in/yaml.v2"

	"github.com/zalando/skipper/dataclients/kubernetes"
)

var errInvalidFixture = errors.New("invalid fixture")

type TestAPIOptions struct {
	FailOn             []string `yaml:"failOn"`
	FindNot            []string `yaml:"findNot"`
	DisableRouteGroups bool     `yaml:"disableRouteGroups"`
}

type namespace struct {
	services       []byte
	ingresses      []byte
	routeGroups    []byte
	endpoints      []byte
	endpointslices []byte
	secrets        []byte
}

type api struct {
	failOn       map[string]bool
	findNot      map[string]bool
	namespaces   map[string]namespace
	all          namespace
	pathRx       *regexp.Regexp
	resourceList []byte
}

func NewAPI(o TestAPIOptions, specs ...io.Reader) (*api, error) {
	a := &api{
		namespaces: make(map[string]namespace),
		// see https://kubernetes.io/docs/reference/using-api/api-concepts/#resource-uris
		pathRx: regexp.MustCompile(
			"(?:/namespaces/([^/]+))?/(services|ingresses|routegroups|endpointslices|endpoints|secrets)(?:/(.+))?",
		),
	}

	var clr kubernetes.ClusterResourceList
	if !o.DisableRouteGroups {
		clr.Items = append(clr.Items, &kubernetes.ClusterResource{Name: kubernetes.RouteGroupsName})
	}

	a.failOn = mapStrings(o.FailOn)
	a.findNot = mapStrings(o.FindNot)

	clrb, err := json.Marshal(clr)
	if err != nil {
		return nil, err
	}

	a.resourceList = clrb

	namespaces := make(map[string]map[string][]any)
	all := make(map[string][]any)

	addObject := func(o map[any]any) error {
		kind, ok := o["kind"].(string)
		if !ok {
			return errInvalidFixture
		}

		meta, ok := o["metadata"].(map[any]any)
		if !ok {
			return errInvalidFixture
		}

		namespace, ok := meta["namespace"]
		if !ok || namespace == "" {
			namespace = "default"
		} else {
			if _, ok := namespace.(string); !ok {
				return errInvalidFixture
			}
		}

		ns := namespace.(string)
		if _, ok := namespaces[ns]; !ok {
			namespaces[ns] = make(map[string][]any)
		}

		namespaces[ns][kind] = append(namespaces[ns][kind], o)
		all[kind] = append(all[kind], o)

		return nil
	}

	for _, spec := range specs {
		d := yaml.NewDecoder(spec)
		for {
			var o map[any]any
			if err := d.Decode(&o); err == io.EOF || err == nil && len(o) == 0 {
				break
			} else if err != nil {
				return nil, err
			}

			kind, ok := o["kind"].(string)
			if !ok {
				return nil, errInvalidFixture
			}

			if kind == "List" {
				items, ok := o["items"].([]any)
				if !ok {
					return nil, errInvalidFixture
				}
				for _, item := range items {
					o, ok := item.(map[any]any)
					if !ok {
						return nil, errInvalidFixture
					}
					if err := addObject(o); err != nil {
						return nil, err
					}
				}
			} else {
				if err := addObject(o); err != nil {
					return nil, err
				}
			}
		}
	}

	for ns, kinds := range namespaces {
		var err error
		a.namespaces[ns], err = initNamespace(kinds)
		if err != nil {
			return nil, err
		}
	}

	a.all, err = initNamespace(all)
	if err != nil {
		return nil, err
	}

	return a, nil
}

func (a *api) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if a.failOn[r.URL.Path] {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if a.findNot[r.URL.Path] {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	if r.URL.Path == kubernetes.ZalandoResourcesClusterURI {
		w.Write(a.resourceList)
		return
	}

	parts := a.pathRx.FindStringSubmatch(r.URL.Path)
	if len(parts) == 0 {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	ns := a.all
	if parts[1] != "" {
		ns = a.namespaces[parts[1]]
	}

	resourceType, name := parts[2], parts[3]
	switch resourceType {
	case "services":
		serve(w, r, ns.services, name)
	case "ingresses":
		serve(w, r, ns.ingresses, name)
	case "routegroups":
		serve(w, r, ns.routeGroups, name)
	case "endpoints":
		serve(w, r, ns.endpoints, name)
	case "endpointslices":
		serve(w, r, ns.endpointslices, name)
	case "secrets":
		serve(w, r, ns.secrets, name)
	default:
		http.Error(w, fmt.Sprintf("unsupported resource type %s", resourceType), http.StatusBadRequest)
	}
}

// Parses an optional parameter with `label selectors` into a map if present or, if not present, returns nil.
func parseSelectors(r *http.Request) map[string]string {
	rawSelector := r.URL.Query().Get("labelSelector")
	if rawSelector == "" {
		return nil
	}

	selectors := map[string]string{}
	for selector := range strings.SplitSeq(rawSelector, ",") {
		kv := strings.Split(selector, "=")
		selectors[kv[0]] = kv[1]
	}

	return selectors
}

func serve(w http.ResponseWriter, r *http.Request, resources []byte, name string) {
	selectors := parseSelectors(r)
	if name == "" && len(selectors) == 0 {
		w.Write(resources)
		return
	}

	itemsMetadata := struct {
		Items []struct {
			Metadata struct {
				Name   string            `json:"name"`
				Labels map[string]string `json:"labels"`
			} `json:"metadata"`
		} `json:"items"`
	}{}

	if err := json.Unmarshal(resources, &itemsMetadata); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// every resource but top level is deserialized because we need access to the indexed array
	allItems := struct {
		Items []any `json:"items"`
	}{}

	if err := json.Unmarshal(resources, &allItems); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// serve named resource if present
	if name != "" {
		for idx, item := range itemsMetadata.Items {
			if item.Metadata.Name == name {
				if result, err := json.Marshal(allItems.Items[idx]); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
				} else {
					w.Write(result)
				}
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// go over each item's label and check if all selectors with their values are present
	var filteredItems []any
	for idx, item := range itemsMetadata.Items {
		allMatch := true
		for k, v := range selectors {
			label, ok := item.Metadata.Labels[k]
			allMatch = allMatch && ok && label == v
		}
		if allMatch {
			filteredItems = append(filteredItems, allItems.Items[idx])
		}
	}

	var result []byte
	if err := itemsJSON(&result, filteredItems); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	} else {
		w.Write(result)
	}
}

func initNamespace(kinds map[string][]any) (ns namespace, err error) {
	if err = itemsJSON(&ns.services, kinds["Service"]); err != nil {
		return
	}

	if err = itemsJSON(&ns.ingresses, kinds["Ingress"]); err != nil {
		return
	}

	if err = itemsJSON(&ns.routeGroups, kinds["RouteGroup"]); err != nil {
		return
	}

	if err = itemsJSON(&ns.endpoints, kinds["Endpoints"]); err != nil {
		return
	}

	if err = itemsJSON(&ns.endpointslices, kinds["EndpointSlice"]); err != nil {
		return
	}

	if err = itemsJSON(&ns.secrets, kinds["Secret"]); err != nil {
		return
	}

	return
}

func itemsJSON(b *[]byte, o []any) error {
	items := map[string]any{"items": o}

	// converting back to YAML, because we have YAMLToJSON() for bytes, and
	// the data in `o` contains YAML parser style keys of type interface{}
	y, err := yaml.Marshal(items)
	if err != nil {
		return err
	}

	*b, err = yaml2.YAMLToJSON(y)
	return err
}

func readAPIOptions(r io.Reader) (o TestAPIOptions, err error) {
	var b []byte
	b, err = io.ReadAll(r)
	if err != nil {
		return
	}

	err = yaml.Unmarshal(b, &o)
	return
}

func mapStrings(s []string) map[string]bool {
	m := make(map[string]bool)
	for _, si := range s {
		m[si] = true
	}

	return m
}
