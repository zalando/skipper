package auth

import (
	"os"
	"testing"

	"github.com/AlexanderYastrebov/noleak"
	"github.com/zalando/skipper/net"
)

func TestMain(m *testing.M) {
	os.Exit(noleak.CheckMainFunc(func() int {
		code := m.Run()
		cleanupAuthClients()
		return code
	}))
}

func cleanupAuthClients() {
	for _, c := range tokeninfoAuthClient {
		if ac, ok := c.(*authClient); ok {
			ac.Close()
		} else if cc, ok := c.(*tokeninfoCache); ok {
			cc.client.(*authClient).Close()
		}
	}

	for _, c := range issuerAuthClient {
		c.Close()
	}

	for _, c := range webhookAuthClient {
		c.Close()
	}

	for _, j := range jwksMap {
		j.EndBackground()
	}

	distributedClaimsClients.Range(func(key, value any) bool {
		value.(*net.Client).Close()
		return true
	})
}
