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
	AddrUpdater      func(namespace, name string) ([]byte, error)
	DefaultNamespace string
	DefaultName      string
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

	ns := r.PathValue("namespace")
	name := r.PathValue("name")

	// fall back to operator-configured defaults (backwards-compatible path)
	if ns == "" {
		ns = rh.DefaultNamespace
	}
	if name == "" {
		name = rh.DefaultName
	}

	if ns == "" || name == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	w.Header().Add("Content-Type", "application/json")
	address, err := rh.AddrUpdater(ns, name)
	if err != nil {
		log.Errorf("Failed to update address for redis handler %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Write(address)
}

func getRedisAddresses(opts *skipper.Options, kdc *kubernetes.Client, m metrics.Metrics) func(namespace, name string) ([]byte, error) {
	return func(namespace, name string) ([]byte, error) {
		a := kdc.GetEndpointAddresses("", namespace, name)
		log.Debugf("Redis updater called for %s/%s and found %d redis endpoints: %v", namespace, name, len(a), a)
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
