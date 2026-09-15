package bottle

import (
	"strings"
	"testing"
)

// TestVNetSWBaseUsesRequestDirectory. The <base> this worker injects governs
// every relative link on a vnet-served page. Pinned to the port root it is
// right only for a one-page server and silently wrong below the top: a page at
// /vnet/8002/cli/README.md linking to "dmsg/README.md" landed on
// /vnet/8002/dmsg/README.md, having dropped the "cli/". Measured on skywire's
// doc serve, where only the landing page's links worked.
func TestVNetSWBaseUsesRequestDirectory(t *testing.T) {
	sw := string(VNetSWJS())
	if !strings.Contains(sw, "function rewriteBase(html, port, path)") {
		t.Fatal("rewriteBase no longer takes the request path — the base cannot be depth-correct without it")
	}
	if !strings.Contains(sw, "p.lastIndexOf('/')") {
		t.Error("rewriteBase does not derive the directory from the request path")
	}
	if strings.Contains(sw, "const href = PREFIX + port + '/';") {
		t.Error("rewriteBase is pinning the base to the port root again")
	}
}

// TestVNetSWIsolationIsOptIn. COEP require-corp is inherited by embedded
// DOCUMENTS, so a vnet-served page inside a cross-origin-isolated parent is
// refused unless it carries COEP itself. The same header then demands CORP of
// that page's own cross-origin subresources, so it must stay opt-in: pages
// served here legitimately pull remote images.
func TestVNetSWIsolationIsOptIn(t *testing.T) {
	sw := string(VNetSWJS())
	for _, h := range []string{
		"Cross-Origin-Embedder-Policy",
		"Cross-Origin-Opener-Policy",
		"Cross-Origin-Resource-Policy",
	} {
		if !strings.Contains(sw, h) {
			t.Errorf("vnet-sw.js cannot set %s — an isolated parent will refuse its frames", h)
		}
	}
	if !strings.Contains(sw, "searchParams.get('coi')") {
		t.Error("vnet-sw.js does not read the coi flag — the headers would be unconditional")
	}
	if !strings.Contains(sw, "if (COI) {") {
		t.Error("the isolation headers are not guarded by the coi flag")
	}
	// The page half has to send it, or the flag is never on.
	if !strings.Contains(string(VNetJS()), "crossOriginIsolated ? '&coi=1'") {
		t.Error("vnet.js does not pass coi=1 when the registering page is isolated")
	}
}
