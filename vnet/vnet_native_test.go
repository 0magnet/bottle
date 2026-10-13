//go:build !(js && wasm)

// Package vnet vnet/vnet_native_test.go
package vnet

import (
	"testing"
	"time"
)

// Natively both namespaces are the OS.
func TestNativeNamespacesReachTheOS(t *testing.T) {
	for _, prefix := range []string{"", VnetPrefix, HostPrefix} {
		l, err := Listen("tcp", prefix+"127.0.0.1:0")
		if err != nil {
			t.Fatalf("%q: %v", prefix, err)
		}
		c, err := DialTimeout("tcp", prefix+l.Addr().String(), time.Second)
		if err != nil {
			t.Fatalf("%q dial: %v", prefix, err)
		}
		if err := c.Close(); err != nil {
			t.Errorf("%q close conn: %v", prefix, err)
		}
		if err := l.Close(); err != nil {
			t.Errorf("%q close listener: %v", prefix, err)
		}
	}
}
