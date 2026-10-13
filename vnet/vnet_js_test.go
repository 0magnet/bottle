//go:build js && wasm

// Package vnet vnet/vnet_js_test.go
package vnet

import (
	"errors"
	"net"
	"testing"
	"time"
)

type fakeHost struct{ listened, dialed string }

func (f *fakeHost) Listen(_, address string) (net.Listener, error) {
	f.listened = address
	return nil, errors.New("fake")
}

func (f *fakeHost) DialTimeout(_, address string, _ time.Duration) (net.Conn, error) {
	f.dialed = address
	return nil, errors.New("fake")
}

func TestHostAddressesGoToTheHostNetwork(t *testing.T) {
	SetHost(nil)
	if _, err := Listen("tcp", "host:127.0.0.1:1080"); !errors.Is(err, ErrNoHost) {
		t.Fatalf("no host network: got %v, want ErrNoHost", err)
	}
	f := &fakeHost{}
	SetHost(f)
	defer SetHost(nil)
	// The fake refuses both, so each must come back with its error.
	if _, err := Listen("tcp", "host:0.0.0.0:1080"); err == nil {
		t.Fatal("listen through the fake host: no error")
	}
	if _, err := DialTimeout("tcp", "host:127.0.0.1:22", time.Second); err == nil {
		t.Fatal("dial through the fake host: no error")
	}
	if f.listened != "0.0.0.0:1080" || f.dialed != "127.0.0.1:22" {
		t.Fatalf("host network got listen %q dial %q", f.listened, f.dialed)
	}
}
