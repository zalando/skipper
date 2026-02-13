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

type RedisHandler struct {
	AddrUpdater func() ([]byte, error)
}

type RedisEndpoint struct {
	Address string `json:"address"`
}

type RedisEndpoints struct {
	Endpoints []RedisEndpoint `json:"endpoints"`
}

func (rh *RedisHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	w.Header().Add("Content-Type", "application/json")
	address, err := rh.AddrUpdater()
	if err != nil {
		log.Errorf("Failed to update address for redis handler %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Write(address)
}

func getRedisAddresses(opts *skipper.Options, kdc *kubernetes.Client, m metrics.Metrics) func() ([]byte, error) {
	return func() ([]byte, error) {
		a := kdc.GetEndpointAddresses("", opts.KubernetesRedisServiceNamespace, opts.KubernetesRedisServiceName)
		log.Debugf("Redis updater called and found %d redis endpoints: %v", len(a), a)
		m.UpdateGauge("redis_endpoints", float64(len(a)))

		result := RedisEndpoints{
			Endpoints: make([]RedisEndpoint, len(a)),
		}
		port := strconv.Itoa(opts.KubernetesRedisServicePort)
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
