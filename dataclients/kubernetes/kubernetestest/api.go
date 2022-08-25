package kubernetestest

import (
	"encoding/json"
	"errors"
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
	services    []byte
	ingresses   []byte
	routeGroups []byte
	endpoints   []byte
	secrets     []byte
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
		pathRx: regexp.MustCompile(
			"(/namespaces/([^/]+))?/(services|ingresses|routegroups|endpoints|secrets)",
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

	namespaces := make(map[string]map[string][]interface{})
	all := make(map[string][]interface{})

	for _, spec := range specs {
		d := yaml.NewDecoder(spec)
		for {
			var o map[string]interface{}
			if err := d.Decode(&o); err == io.EOF || err == nil && len(o) == 0 {
				break
			} else if err != nil {
				return nil, err
			}

			kind, ok := o["kind"].(string)
			if !ok {
				return nil, errInvalidFixture
			}

			meta, ok := o["metadata"].(map[interface{}]interface{})
			if !ok {
				return nil, errInvalidFixture
			}

			namespace, ok := meta["namespace"]
			if !ok || namespace == "" {
				namespace = "default"
			} else {
				if _, ok := namespace.(string); !ok {
					return nil, errInvalidFixture
				}
			}

			ns := namespace.(string)
			if _, ok := namespaces[ns]; !ok {
				namespaces[ns] = make(map[string][]interface{})
			}

			namespaces[ns][kind] = append(namespaces[ns][kind], o)
			all[kind] = append(all[kind], o)
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
	if parts[2] != "" {
		ns = a.namespaces[parts[2]]
	}

	var b []byte
	switch parts[3] {
	case "services":
		b = filterBySelectors(ns.services, parseSelectors(r))
	case "ingresses":
		b = filterBySelectors(ns.ingresses, parseSelectors(r))
	case "routegroups":
		b = filterBySelectors(ns.routeGroups, parseSelectors(r))
	case "endpoints":
		b = filterBySelectors(ns.endpoints, parseSelectors(r))
	case "secrets":
		b = filterBySelectors(ns.secrets, parseSelectors(r))
	default:
		w.WriteHeader(http.StatusNotFound)
		return
	}

	w.Write(b)
}

// Parses an optional parameter with `label selectors` into a map if present or, if not present, returns nil.
func parseSelectors(r *http.Request) map[string]string {
	rawSelector := r.URL.Query().Get("labelSelector")
	if rawSelector == "" {
		return nil
	}

	selectors := map[string]string{}
	for _, selector := range strings.Split(rawSelector, ",") {
		kv := strings.Split(selector, "=")
		selectors[kv[0]] = kv[1]
	}

	return selectors
}

// Filters all resources that are already set in k8s namespace using the given selectors map.
// All resources are initially set to `namespace` as slices of bytes and for most tests it's not needed to make it any more complex.
// This helper function deserializes resources, finds a metadata with labels in them and check if they have all
// requested labels. If they do, they are returned.
func filterBySelectors(resources []byte, selectors map[string]string) []byte {
	if len(selectors) == 0 {
		return resources
	}

	labels := struct {
		Items []struct {
			Metadata struct {
				Labels map[string]string `json:"labels"`
			} `json:"metadata"`
		} `json:"items"`
	}{}

	// every resource but top level is deserialized because we need access to the indexed array
	allItems := struct {
		Items []interface{} `json:"items"`
	}{}

	if json.Unmarshal(resources, &labels) != nil || json.Unmarshal(resources, &allItems) != nil {
		return resources
	}

	// go over each item's label and check if all selectors with their values are present
	var filteredItems []interface{}
	for idx, item := range labels.Items {
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
		return resources
	}

	return result
}

func initNamespace(kinds map[string][]interface{}) (ns namespace, err error) {
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

	if err = itemsJSON(&ns.secrets, kinds["Secret"]); err != nil {
		return
	}

	return
}

func itemsJSON(b *[]byte, o []interface{}) error {
	items := map[string]interface{}{"items": o}

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
