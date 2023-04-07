package routesrv

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/dataclients/kubernetes"
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
		log.Errorf("Could not update address for redis handler %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Write(address)
}

func getRedisAddresses(namespace, name string, kdc *kubernetes.Client) func() ([]byte, error) {
	return func() ([]byte, error) {
		result := RedisEndpoints{}
		a := kdc.GetEndpointAddresses(namespace, name)
		log.Infof("Redis updater called and found %d redis endpoints", len(a))
		for i := 0; i < len(a); i++ {
			a[i] = strings.TrimPrefix(a[i], "TCP://")
			result.Endpoints = append(result.Endpoints, RedisEndpoint{Address: a[i]})
		}
		data, err := json.Marshal(result)

		if err != nil {
			return nil, fmt.Errorf("failed to marshal json data %w", err)
		}

		return data, nil
	}
}
