package bottle

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// fakeIDB is just enough IndexedDB for jsfs.persist, kept in a file so a
// second node process is a page reload.
const fakeIDB = `
const v8 = require('v8'), nodefs = require('fs');
const ENV = process.env, DB = ENV.IDB_FILE; // jsfs replaces process
const later = (f) => setTimeout(f, 0);
globalThis.indexedDB = {
	open() {
		const rq = {};
		later(() => {
			const store = {
				put(v) { nodefs.writeFileSync(DB, v8.serialize(v)); },
				get() { return nodefs.existsSync(DB) ? v8.deserialize(nodefs.readFileSync(DB)) : undefined; },
			};
			rq.result = {
				createObjectStore() {},
				transaction() {
					const tx = {
						objectStore() {
							return {
								put(v) { store.put(v); later(() => tx.oncomplete && tx.oncomplete()); },
								get() { const r = {}; later(() => { r.result = store.get(); r.onsuccess(); }); return r; },
								delete() { later(() => tx.oncomplete && tx.oncomplete()); },
							};
						},
					};
					return tx;
				},
			};
			if (!nodefs.existsSync(DB) && rq.onupgradeneeded) rq.onupgradeneeded();
			rq.onsuccess();
		});
		return rq;
	},
};
`

// The page seeds /etc/site each load; only /home is persisted.
const persistScript = `
jsfs.mkdirp('/etc');
jsfs.writeFile('/etc/site', ENV.LOAD);
const home = (p) => p === '/home' || p.startsWith('/home/');
jsfs.persist.enable('t', { exclude: (p) => !home(p) }).then(({ restored }) => {
	const read = (p) => { const d = jsfs.readFile(p); return d ? new TextDecoder().decode(d) : '-'; };
	if (ENV.LOAD === '1') {
		jsfs.mkdirp('/home/user/.config');
		jsfs.writeFile('/home/user/.config/cart', 'two widgets');
		jsfs.writeFile('/tmp/scratch', 'old');
		return jsfs.persist.flush().then(() => console.log('saved restored=' + restored));
	}
	const tmp = jsfs.sync.stat('/tmp').isDirectory();
	console.log('restored=' + restored + ' cart=' + read('/home/user/.config/cart') + ' site=' + read('/etc/site') + ' tmp=' + tmp + ' scratch=' + read('/tmp/scratch'));
});
`

// TestPersistKeepsWhatItDoesNotCover: a page that persists only /home gets
// /home back on reload, and everything else as this load made it: its own
// seeded files, and a /tmp that exists and is empty. The restore once
// replaced the whole tree, leaving no /tmp at all.
func TestPersistKeepsWhatItDoesNotCover(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "persist.js")
	if err := os.WriteFile(script, []byte(fakeIDB+string(JSFS())+persistScript), 0o600); err != nil {
		t.Fatal(err)
	}
	run := func(load string) string {
		cmd := exec.Command(node, script) //nolint:gosec
		cmd.Env = append(os.Environ(), "IDB_FILE="+filepath.Join(dir, "idb"), "LOAD="+load)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("node: %v\n%s", err, out)
		}
		return strings.TrimSpace(string(out))
	}
	if got := run("1"); got != "saved restored=false" {
		t.Fatalf("first load: %s", got)
	}
	if got, want := run("2"), "restored=true cart=two widgets site=2 tmp=true scratch=-"; got != want {
		t.Errorf("reload: %s\nwant         %s", got, want)
	}
}
