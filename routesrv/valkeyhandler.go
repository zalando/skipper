package routesrv

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper"
	"github.com/zalando/skipper/dataclients/kubernetes"
	"github.com/zalando/skipper/metrics"
)

type ValkeyHandler struct {
	AddrUpdater      func(namespace, name string) ([]byte, error)
	DefaultNamespace string
	DefaultName      string
}

type ValkeyEndpoint struct {
	Address string `json:"address"`
}

type ValkeyEndpoints struct {
	Endpoints []ValkeyEndpoint `json:"endpoints"`
}

func (vh *ValkeyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	ns := r.PathValue("namespace")
	name := r.PathValue("name")

	// fall back to operator-configured defaults (backwards-compatible path)
	if ns == "" {
		ns = vh.DefaultNamespace
	}
	if name == "" {
		name = vh.DefaultName
	}

	if ns == "" || name == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	w.Header().Add("Content-Type", "application/json")
	address, err := vh.AddrUpdater(ns, name)
	if err != nil {
		log.Errorf("Failed to update address for valkey handler %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Write(address)
}

func getValkeyAddresses(opts *skipper.Options, kdc *kubernetes.Client, m metrics.Metrics) func(namespace, name string) ([]byte, error) {
	return func(namespace, name string) ([]byte, error) {
		a := kdc.GetEndpointAddresses("", namespace, name)
		log.Debugf("Valkey updater called for %s/%s and found %d valkey endpoints: %v", namespace, name, len(a), a)
		m.UpdateGauge("valkey_endpoints", float64(len(a)))

		result := ValkeyEndpoints{
			Endpoints: make([]ValkeyEndpoint, len(a)),
		}
		port := strconv.Itoa(opts.KubernetesValkeyServicePort)
		for i := range a {
			result.Endpoints[i].Address = net.JoinHostPort(a[i], port)
		}

		data, err := json.Marshal(result)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal json data %w", err)
		}
		return data, nil
	}
}
