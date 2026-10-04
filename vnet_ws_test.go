package bottle

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A minimal WebSocket echo server on a vnet port, written against vnet's own
// conn API, drives vnet.webSocket through the handshake, a masked text and a
// binary frame each way, and the closing handshake.
const vnetWSScript = `
const v = globalThis.vnet;
const te = new TextEncoder(), td = new TextDecoder();
v.listen(9000, (id) => {
	let buf = new Uint8Array(0), up = false;
	const append = (b) => { const n = new Uint8Array(buf.length + b.length); n.set(buf); n.set(b, buf.length); buf = n; };
	const frame = (op, p) => { const h = new Uint8Array([0x80 | op, p.length]); v.send(id, 'b', h); v.send(id, 'b', p); };
	const pump = () => {
		let b;
		while ((b = v.recv(id, 'b')) !== null) append(b);
		if (!up) {
			const s = td.decode(buf), end = s.indexOf('\r\n\r\n');
			if (end < 0) { v.onReadable(id, 'b', pump); return; }
			const proto = (s.match(/Sec-WebSocket-Protocol: ([^\r\n,]+)/) || [])[1] || '';
			v.send(id, 'b', te.encode('HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Protocol: ' + proto + '\r\n\r\n'));
			buf = buf.slice(end + 4); up = true;
		}
		while (buf.length >= 6) {
			const op = buf[0] & 15, masked = (buf[1] & 0x80) !== 0, n = buf[1] & 127;
			if (!masked) { console.log('FAIL client frame not masked'); return; }
			if (buf.length < 6 + n) break;
			const mk = buf.slice(2, 6), p = buf.slice(6, 6 + n);
			for (let i = 0; i < n; i++) p[i] ^= mk[i & 3];
			buf = buf.slice(6 + n);
			if (op === 8) { frame(8, p); v.close(id, 'b'); return; }
			if (op === 1) frame(1, te.encode('echo:' + td.decode(p)));
			if (op === 2) frame(2, p.map((x) => x + 1));
		}
		v.onReadable(id, 'b', pump);
	};
	v.onReadable(id, 'b', pump);
});
const out = [];
const ws = v.webSocket(9000, '/sock?x=1', ['chat']);
ws.binaryType = 'arraybuffer';
ws.onopen = () => { out.push('open proto=' + ws.protocol + ' state=' + ws.readyState); ws.send('hi'); };
let n = 0;
ws.onmessage = (ev) => {
	n++;
	if (typeof ev.data === 'string') { out.push('text=' + ev.data); ws.send(new Uint8Array([1, 2, 3])); }
	else { out.push('bin=' + Array.from(new Uint8Array(ev.data)).join(',')); ws.close(1000, 'bye'); }
};
ws.addEventListener('close', (ev) => {
	out.push('close code=' + ev.code + ' clean=' + ev.wasClean + ' state=' + ws.readyState);
	console.log(out.join(' '));
	clearTimeout(timer);
});
const refused = v.webSocket(9001, '/');
refused.onerror = () => out.push('refused=error');
const timer = setTimeout(() => console.log('TIMEOUT ' + out.join(' ')), 3000);
`

func TestVNetWebSocket(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	script := filepath.Join(t.TempDir(), "ws.js")
	if err := os.WriteFile(script, append(append([]byte{}, VNetJS()...), vnetWSScript...), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(node, script).CombinedOutput() //nolint:gosec
	if err != nil {
		t.Fatalf("node: %v\n%s", err, out)
	}
	got := strings.SplitN(strings.TrimSpace(string(out)), "\n", 2)[0]
	want := "refused=error open proto=chat state=1 text=echo:hi bin=2,3,4 close code=1000 clean=true state=3"
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}
