package bottle

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// jsfsMountScript drives jsfs's mount layer the way Go's syscall/fs_js does,
// through callback-style fs calls, against a provider that answers later.
const jsfsMountScript = `
const files = new Map([['/a.txt', new Uint8Array([104, 105])]]);
const dirs = new Set(['/']);
const handles = new Map(); let nh = 1;
const st = (mode, size) => ({ dev: 9, ino: 1, mode, nlink: 1, uid: 0, gid: 0, rdev: 0, size,
	blksize: 4096, blocks: 1, atimeMs: 0, mtimeMs: 0, ctimeMs: 0 });
const later = (f) => setTimeout(f, 1);
const provider = {
	stat(p, cb) { later(() => dirs.has(p) ? cb(null, st(0o40755, 64)) : files.has(p) ? cb(null, st(0o100644, files.get(p).length)) : cb({ code: 'ENOENT' })); },
	readdir(p, cb) { later(() => cb(null, [...files.keys()].map((k) => k.slice(1)))); },
	open(p, flags, mode, cb) { later(() => { if (!files.has(p)) files.set(p, new Uint8Array(0)); const h = nh++; handles.set(h, { p, pos: 0 }); cb(null, h); }); },
	read(h, len, pos, cb) { later(() => { const e = handles.get(h); const d = files.get(e.p); const at = pos === null ? e.pos : pos; const b = d.subarray(at, at + len); if (pos === null) e.pos += b.length; cb(null, b); }); },
	write(h, bytes, pos, cb) { later(() => { const e = handles.get(h); files.set(e.p, bytes); cb(null, bytes.length); }); },
	close(h, cb) { later(() => { handles.delete(h); cb(null); }); },
	unmounted() { globalThis.wasUnmounted = true; },
};
const P = (f, ...a) => new Promise((res, rej) => fs[f](...a, (err, v) => err ? rej(err) : res(v)));
(async () => {
	jsfs.mount('/mnt/peer', provider);
	const out = [];
	out.push('parent=' + (await P('readdir', '/mnt')).join(','));
	out.push('isdir=' + (await P('stat', '/mnt/peer')).isDirectory());
	out.push('list=' + (await P('readdir', '/mnt/peer')).join(','));
	const fd = await P('open', '/mnt/peer/a.txt', 0, 0);
	const buf = new Uint8Array(8);
	const n = await P('read', fd, buf, 0, 8, null);
	out.push('read=' + Buffer.from(buf.subarray(0, n)).toString());
	await P('close', fd);
	const wfd = await P('open', '/mnt/peer/b.txt', 0o101, 0o644);
	await P('write', wfd, new Uint8Array([111, 107]), 0, 2, null);
	await P('close', wfd);
	out.push('b=' + Buffer.from(files.get('/b.txt')).toString());
	try { await P('rename', '/mnt/peer/b.txt', '/tmp/b.txt'); out.push('rename=ok'); } catch (e) { out.push('rename=' + e.code); }
	try { await P('mkdir', '/mnt/peer/d', 0o755); } catch (e) { out.push('mkdir=' + e.code); }
	try { await P('stat', '/mnt/peer/none'); } catch (e) { out.push('missing=' + e.code + ',' + (e instanceof Error)); }
	const open = await P('open', '/mnt/peer/a.txt', 0, 0);
	jsfs.unmount('/mnt/peer');
	try { await P('read', open, buf, 0, 8, null); } catch (e) { out.push('after=' + e.code); }
	out.push('unmounted=' + !!globalThis.wasUnmounted);
	out.push('mounts=' + jsfs.mounts().length);
	console.log(out.join(' '));
})().catch((e) => { console.log('FAIL ' + e.code + ' ' + e.message); });
`

func TestJSFSMount(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	script := filepath.Join(t.TempDir(), "mount.js")
	if err := os.WriteFile(script, append(append([]byte{}, JSFS()...), jsfsMountScript...), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(node, script).CombinedOutput() //nolint:gosec
	if err != nil {
		t.Fatalf("node: %v\n%s", err, out)
	}
	got := strings.TrimSpace(string(out))
	want := "parent=peer isdir=true list=a.txt read=hi b=ok rename=EXDEV mkdir=ENOSYS missing=ENOENT,true after=EIO unmounted=true mounts=0"
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}
