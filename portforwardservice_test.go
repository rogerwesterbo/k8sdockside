package main

import (
	"errors"
	"testing"
)

func TestPortTakenRecognisesAPortSomethingElseHolds(t *testing.T) {
	// The wording differs between platforms and between the listener and the
	// forwarder above it, so all three are matched. Getting this wrong means a
	// forward that asked for "any free port" refuses to move off the one it was
	// last given.
	for _, err := range []error{
		errors.New("unable to create listener: Error listen tcp4 127.0.0.1:8080: bind: address already in use"),
		errors.New("unable to listen on any of the requested ports: [{8080 80}]"),
		errors.New("Only one usage of each socket address is normally permitted"),
	} {
		if !portTaken(err) {
			t.Errorf("portTaken(%v) = false, want true", err)
		}
	}
}

func TestPortTakenLeavesOtherFailuresAlone(t *testing.T) {
	// Retrying these on another port would only fail again, having hidden the
	// real reason behind a second attempt.
	for _, err := range []error{
		nil,
		errors.New(`pods "web" is forbidden: User "dev" cannot create resource "pods/portforward"`),
		errors.New("service web has no running pod behind it to forward to"),
	} {
		if portTaken(err) {
			t.Errorf("portTaken(%v) = true, want false", err)
		}
	}
}
