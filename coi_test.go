package bottle

import (
	"strings"
	"testing"
)

// The COI pair is what makes SharedArrayBuffer — and therefore fsbridge and
// off-thread compiles — reachable on a static host that cannot set headers.
// Both halves are needed and each is useless alone, so assert they ship
// together and carry the three headers isolation actually requires.
func TestCOIScriptsShipTogether(t *testing.T) {
	sw := string(COISWJS())
	reg := string(COIRegisterJS())
	if sw == "" || reg == "" {
		t.Fatal("a COI half is empty: the pair is useless without both")
	}
	for _, h := range []string{
		"Cross-Origin-Embedder-Policy",
		"Cross-Origin-Opener-Policy",
		"Cross-Origin-Resource-Policy",
	} {
		if !strings.Contains(sw, h) {
			t.Errorf("coi-sw.js does not set %s; isolation needs all three", h)
		}
	}
	// The reload is the one dangerous part — an unguarded one loops forever on
	// a browser that will not isolate. Check the guard is present rather than
	// trusting the comment.
	if !strings.Contains(reg, "coi-sw-reloaded") {
		t.Error("coi-register.js has no session guard: an unguarded reload loops")
	}
	if !strings.Contains(reg, "crossOriginIsolated") {
		t.Error("coi-register.js does not short-circuit when already isolated")
	}
	if !strings.Contains(reg, "coi-sw.js") {
		t.Error("coi-register.js does not register coi-sw.js")
	}
}
