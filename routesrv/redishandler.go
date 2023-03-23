package routesrv

import (
	"encoding/json"
	"net/http"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/dataclients/kubernetes"
)

type RedisHandler struct {
	AddrUpdater func() []byte
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
	w.Write(rh.AddrUpdater())
}

func getRedisAddresses(namespace, name string, kdc *kubernetes.Client) func() []byte {
	return func() []byte {
		endpoints := make([]RedisEndpoint, 0)
		RedisEndpoints := RedisEndpoints{Endpoints: endpoints}
		a := kdc.GetEndpointAddresses(namespace, name)
		log.Infof("Redis updater called and found %d redis endpoints", len(a))
		for i := 0; i < len(a); i++ {
			a[i] = strings.TrimPrefix(a[i], "TCP://")
			RedisEndpoints.Endpoints = append(RedisEndpoints.Endpoints, RedisEndpoint{Address: a[i]})
		}
		data, err := json.Marshal(RedisEndpoints)

		if err != nil {
			log.Errorf("Failed to marshal json data %v", err)
			return nil
		}

		return data
	}
}
