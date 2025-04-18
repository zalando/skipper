package main

import (
	"testing"
)

func TestGetCommandSuccess(t *testing.T) {
	_, err := getCommand([]string{"some", "print"})
	if err != nil {
		t.Error("print is a valid command")
	}
}

func TestGetCommandFail(t *testing.T) {
	_, err := getCommand([]string{"some", "hello"})
	if err != errInvalidCommand {
		t.Error("hello is an invalid command")
	}
}

func TestGetCommandEmpty(t *testing.T) {
	_, err := getCommand([]string{"some"})
	if err != errMissingCommand {
		t.Error("empty should fail ")
	}
}
