package tee

import (
	"os"
	"testing"

	"github.com/AlexanderYastrebov/noleak"
)

func TestMain(m *testing.M) {
	os.Exit(noleak.CheckMainFunc(func() int {
		code := m.Run()
		cleanupClients()
		return code
	}))
}

func cleanupClients() {
	teeClients.mu.Lock()
	for _, c := range teeClients.store {
		c.Close()
	}
	teeClients.mu.Unlock()

	teeResponseClients.mu.Lock()
	for _, c := range teeResponseClients.store {
		c.Close()
	}
	teeResponseClients.mu.Unlock()
}
