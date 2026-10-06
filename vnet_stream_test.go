package bottle

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestVNetSWStreamsAnEndlessBody drives vnet.js's service-worker responder
// under node with a server that sends a head and then keeps writing. The head
// and the first pieces must reach the worker before any end, as an event
// stream needs; a buffered reply never arrives at all.
func TestVNetSWStreamsAnEndlessBody(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	src, err := filepath.Abs("vnet.js")
	if err != nil {
		t.Fatal(err)
	}
	script := `
let handler = null;
Object.defineProperty(globalThis, 'navigator', { configurable: true, value: { serviceWorker: {
	addEventListener(type, fn) { if (type === 'message') handler = fn; },
	register() { return Promise.reject(new Error('no worker here')); },
} } });
require(process.argv[2]);
const v = globalThis.vnet;
const te = new TextEncoder();
v.listen(9001, (id) => {
	const tick = () => {
		v.send(id, 'b', te.encode('data: ' + Date.now() + '\n\n'));
		setTimeout(tick, 20);
	};
	v.onReadable(id, 'b', () => {
		while (v.recv(id, 'b')) { /* drain the request */ }
		v.send(id, 'b', te.encode('HTTP/1.0 200 OK\r\nContent-Type: text/event-stream\r\n\r\n'));
		tick();
	});
});
v.enableSW().catch(() => {});
const seen = [];
const ch = new MessageChannel();
ch.port2.onmessage = (ev) => {
	const m = ev.data;
	seen.push(m.head ? 'head:' + m.status + ':' + m.headers['content-type'] : m.chunk ? 'chunk' : m.end ? 'end' : '?');
	if (seen.filter((s) => s === 'chunk').length >= 3) {
		ch.port2.postMessage({ cancel: true });
		console.log(seen.join(','));
		process.exit(0);
	}
};
handler({ data: { type: 'vnet-fetch', stream: true, port: 9001, method: 'GET', path: '/sse', headers: {} }, ports: [ch.port1] });
setTimeout(() => { console.log('stalled:' + seen.join(',')); process.exit(1); }, 3000);
`
	f := filepath.Join(t.TempDir(), "stream.js")
	if err := os.WriteFile(f, []byte(script), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(node, f, src).CombinedOutput() //nolint:gosec
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	got := strings.TrimSpace(string(out))
	if !strings.HasPrefix(got, "head:200:text/event-stream,chunk") || strings.Contains(got, "end") {
		t.Fatalf("want the head then chunks with no end, got %q", got)
	}
}
