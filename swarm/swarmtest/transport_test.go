package swarmtest

import (
	"net"
	"testing"
	"time"

	"github.com/hashicorp/memberlist"
)

func TestCustomTransport(t *testing.T) {
	nodeName := "nodeCustom1"
	addrPort, err := net.ResolveUDPAddr("udp", "127.0.0.1:9400")
	if err != nil {
		t.Fatalf("Failed to ResolveUDPAddr: %v", err)
	}
	ipStr := addrPort.IP.String()
	port := addrPort.Port

	cfg := createConfig(nodeName, ipStr, port)
	tr, ok := cfg.Transport.(*CustomNetTransport)
	if !ok {
		t.Fatalf("Failed to get CustomTransport")
	}
	defer tr.Exit()
	list, err := memberlist.Create(cfg)
	defer list.Shutdown()

	if conn, err := tr.DialAddressTimeout(tr.SelfAddress(), 100*time.Millisecond); err != nil {
		t.Fatalf("Failed to DialAddressTimeout: %v", err)
	} else {
		conn.Close()
	}

	if conn, err := tr.DialTimeout(addrPort.String(), 100*time.Millisecond); err != nil {
		t.Fatalf("Failed to DialTimeout: %v", err)
	} else {
		conn.Close()
	}

	if _, _, err := tr.FinalAdvertiseAddr(ipStr, port); err != nil {
		t.Fatalf("Failed to FinalAdvertiseAddr: %v", err)
	}

	if ti, err := tr.WriteToAddress([]byte("hi"), tr.SelfAddress()); err != nil {
		t.Fatalf("Failed to WriteToAddress: %v", err)
	} else {
		d := time.Now().Sub(ti)
		t.Logf("WriteToAddress took %v", d)
	}

	if ti, err := tr.WriteTo([]byte("hi"), tr.SelfAddress().Addr); err != nil {
		t.Fatalf("Failed to WriteTo: %v", err)
	} else {
		d := time.Now().Sub(ti)
		t.Logf("WriteTo took %v", d)
	}

	if err := tr.Shutdown(); err != nil {
		t.Fatalf("Failed to shutdown: %v", err)
	}
}
