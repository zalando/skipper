package queuelistener

import (
	"os"
	"testing"

	"github.com/AlexanderYastrebov/noleak"
)

func TestMain(m *testing.M) {
	os.Exit(noleak.CheckMain(m))
}
