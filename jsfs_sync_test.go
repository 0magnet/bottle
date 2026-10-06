package bottle

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// jsfsSyncScript drives jsfs.sync the way a WASI loader does and checks the
// callback API still sees the same tree.
const jsfsSyncScript = `
const s = jsfs.sync, C = s.constants;
const out = [];
const code = (f) => { try { f(); return 'ok'; } catch (e) { return e.code; } };
s.mkdir('/tmp/d', 0o755);
let fd = s.open('/tmp/d/f', C.O_WRONLY | C.O_CREAT | C.O_TRUNC, 0o644);
const b = new TextEncoder().encode('hello');
out.push('w=' + s.write(fd, b, 0, b.length, null));
s.close(fd);
fd = s.open('/tmp/d/f', C.O_WRONLY | C.O_APPEND, 0);
s.write(fd, b, 0, 1, null);
s.close(fd);
fd = s.open('/tmp/d/f', C.O_RDONLY, 0);
const buf = new Uint8Array(16);
const n = s.read(fd, buf, 0, 16, null);
out.push('r=' + new TextDecoder().decode(buf.subarray(0, n)));
out.push('size=' + s.fstat(fd).size);
s.close(fd);
out.push('excl=' + code(() => s.open('/tmp/d/f', C.O_CREAT | C.O_EXCL | C.O_WRONLY, 0o644)));
s.symlink('f', '/tmp/d/l');
out.push('link=' + s.readlink('/tmp/d/l') + ',' + s.lstat('/tmp/d/l').isSymbolicLink() + ',' + s.stat('/tmp/d/l').isFile());
s.rename('/tmp/d/f', '/tmp/d/g');
out.push('ls=' + s.readdir('/tmp/d').sort().join('+'));
out.push('notempty=' + code(() => s.rmdir('/tmp/d')));
s.truncate('/tmp/d/g', 2);
out.push('trunc=' + new TextDecoder().decode(jsfs.readFile('/tmp/d/g')));
const [r, w] = jsfs.pipe();
out.push('pipe=' + code(() => s.read(r, buf, 0, 4, null)));
s.write(w, b, 0, 2, null);
out.push('piped=' + s.read(r, buf, 0, 4, null));
s.close(w);
out.push('eof=' + s.read(r, buf, 0, 4, null));
jsfs.mount('/mnt/x', { stat(p, cb) { cb(null, { mode: 0o040755, size: 0 }); } });
out.push('mount=' + code(() => s.stat('/mnt/x/a')));
fs.stat('/tmp/d/g', (err, st) => {
	out.push('cb=' + (err ? err.code : st.size));
	console.log(out.join(' '));
});
`

func TestJSFSSync(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	script := filepath.Join(t.TempDir(), "sync.js")
	if err := os.WriteFile(script, append(append([]byte{}, JSFS()...), jsfsSyncScript...), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(node, script).CombinedOutput() //nolint:gosec
	if err != nil {
		t.Fatalf("node: %v\n%s", err, out)
	}
	got := strings.TrimSpace(string(out))
	want := "w=5 r=helloh size=6 excl=EEXIST link=f,true,true ls=g+l notempty=ENOTEMPTY trunc=he pipe=EAGAIN piped=2 eof=0 mount=ENOTSUP cb=2"
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}
