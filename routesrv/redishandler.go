package routesrv

import (
	"net/http"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/dataclients/kubernetes"
)

type RedisHandler struct {
	AddrUpdater func() []byte
}

func (rh *RedisHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	w.Header().Add("Content-Type", "text/plain; charset=utf-8")
	w.Write(rh.AddrUpdater())
}

func getRedisAddresses(namespace, name string, kdc *kubernetes.Client) func() []byte {
	return func() []byte {
		data := make([]byte, 0)
		a := kdc.GetEndpointAddresses(namespace, name)
		log.Infof("Redis updater called and found %d redis endpoints", len(a))
		for i := 0; i < len(a); i++ {
			a[i] = strings.TrimPrefix(a[i], "TCP://")
			data = append(data, []byte(a[i])...)
			data = append(data, []byte("\n")...)
		}
		return data
	}
}
