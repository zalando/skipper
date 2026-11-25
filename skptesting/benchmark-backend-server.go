//go:build ignore

// benchmark-backend-server is a minimal backend server used by the h2c/h2/h3
// benchmark scripts. It serves lorem.html over the requested protocol.
//
// Usage:
//
//	go run benchmark-backend-server.go -proto h2c -addr :9980
//	go run benchmark-backend-server.go -proto h2  -addr :9980 -cert cert.pem -key key.pem
//	go run benchmark-backend-server.go -proto h3  -addr :9980 -cert cert.pem -key key.pem
package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"

	"github.com/quic-go/quic-go/http3"
)

func main() {
	proto := flag.String("proto", "h2c", "protocol: h2c, h2, or h3")
	addr := flag.String("addr", ":9980", "listen address")
	cert := flag.String("cert", "cert.pem", "TLS certificate PEM file (h2, h3)")
	key := flag.String("key", "key.pem", "TLS key PEM file (h2, h3)")
	flag.Parse()

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "lorem.html")
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	switch *proto {
	case "h2c":
		srv := &http.Server{Addr: *addr, Handler: mux}
		srv.Protocols = new(http.Protocols)
		srv.Protocols.SetHTTP1(true)
		srv.Protocols.SetUnencryptedHTTP2(true)
		log.Printf("h2c backend listening on %s", *addr)
		if err := srv.ListenAndServe(); err != nil {
			fmt.Fprintf(os.Stderr, "h2c server: %v\n", err)
			os.Exit(1)
		}

	case "h2":
		tlsCert, err := tls.LoadX509KeyPair(*cert, *key)
		if err != nil {
			fmt.Fprintf(os.Stderr, "load TLS cert: %v\n", err)
			os.Exit(1)
		}
		srv := &http.Server{
			Addr:    *addr,
			Handler: mux,
			TLSConfig: &tls.Config{
				Certificates: []tls.Certificate{tlsCert},
				NextProtos:   []string{"h2", "http/1.1"},
			},
		}
		log.Printf("h2 backend listening on %s", *addr)
		if err := srv.ListenAndServeTLS("", ""); err != nil {
			fmt.Fprintf(os.Stderr, "h2 server: %v\n", err)
			os.Exit(1)
		}

	case "h3":
		tlsCert, err := tls.LoadX509KeyPair(*cert, *key)
		if err != nil {
			fmt.Fprintf(os.Stderr, "load TLS cert: %v\n", err)
			os.Exit(1)
		}
		tlsConf := http3.ConfigureTLSConfig(&tls.Config{
			Certificates: []tls.Certificate{tlsCert},
		})
		conn, err := net.ListenPacket("udp", *addr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "udp listen: %v\n", err)
			os.Exit(1)
		}
		srv := &http3.Server{
			TLSConfig: tlsConf,
			Handler:   mux,
		}
		log.Printf("h3 backend listening on %s (UDP)", *addr)
		if err := srv.Serve(conn); err != nil {
			fmt.Fprintf(os.Stderr, "h3 server: %v\n", err)
			os.Exit(1)
		}

	default:
		fmt.Fprintf(os.Stderr, "unknown protocol %q; use h2c, h2, or h3\n", *proto)
		os.Exit(1)
	}
}
