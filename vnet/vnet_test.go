// Package vnet vnet/vnet_test.go
package vnet

import (
	"testing"
)

func TestSplit(t *testing.T) {
	for in, want := range map[string]struct {
		addr string
		host bool
	}{
		"127.0.0.1:1080":      {"127.0.0.1:1080", false},
		"vnet:127.0.0.1:1080": {"127.0.0.1:1080", false},
		"host:0.0.0.0:1080":   {"0.0.0.0:1080", true},
		"host::1080":          {":1080", true},
	} {
		addr, host := Split(in)
		if addr != want.addr || host != want.host {
			t.Fatalf("Split(%q) = %q, %v; want %q, %v", in, addr, host, want.addr, want.host)
		}
	}
}
