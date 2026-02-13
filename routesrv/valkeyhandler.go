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
	AddrUpdater func() ([]byte, error)
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
	w.Header().Add("Content-Type", "application/json")
	address, err := vh.AddrUpdater()
	if err != nil {
		log.Errorf("Failed to update address for valkey handler %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Write(address)
}

func getValkeyAddresses(opts *skipper.Options, kdc *kubernetes.Client, m metrics.Metrics) func() ([]byte, error) {
	return func() ([]byte, error) {
		a := kdc.GetEndpointAddresses("", opts.KubernetesValkeyServiceNamespace, opts.KubernetesValkeyServiceName)
		log.Debugf("Valkey updater called and found %d valkey endpoints: %v", len(a), a)
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
