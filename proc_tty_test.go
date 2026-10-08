package bottle

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// procTTYPrelude reads the programs before jsfs replaces node's process and
// fs, and runs the page's scripts after it.
const procTTYPrelude = `
const nodefs = require('fs');
const ENV = process.env;
const CHILD = new Uint8Array(nodefs.readFileSync(ENV.CHILD_WASM));
const PARENT = ENV.PARENT_WASM ? new Uint8Array(nodefs.readFileSync(ENV.PARENT_WASM)) : null;
const TINY = ENV.TINY_WASM ? new Uint8Array(nodefs.readFileSync(ENV.TINY_WASM)) : null;
const TINY_EXEC = ENV.TINY_EXEC;
const vm = require('vm');
`

// procTTYScript drives a terminal child through proc.js as a shell would:
// keys in, output out as it is written, a resize, raw mode, the end of its
// input, and a kill of a child that never asked to be interruptible.
const procTTYScript = `
const td = new TextDecoder(), te = new TextEncoder();
jsfs.writeFile('/bin/child', CHILD);
const results = [];
const waitFor = (h, s) => new Promise((res, rej) => {
	const t0 = Date.now();
	const iv = setInterval(() => {
		if (h.text.includes(s)) { clearInterval(iv); res(); }
		else if (Date.now() - t0 > 20000) { clearInterval(iv); rej(new Error('timeout waiting for ' + JSON.stringify(s) + ' in ' + JSON.stringify(h.text))); }
	}, 2);
});
const start = (o) => {
	const h = { text: '', raws: [] };
	h.p = proc.spawn(Object.assign({
		argv: ['child'], env: { PATH: '/bin' }, stdin: 'pipe',
		tty: { cols: 80, rows: 24, onRaw: (on) => h.raws.push(on) },
		stdout: (b) => { h.text += td.decode(b); },
	}, o));
	return h;
};
(async () => {
	// keys, a resize, and q
	let h = start({});
	await waitFor(h, 'ready 80x24\n');
	h.p.stdin.write(te.encode('hello\n'));
	await waitFor(h, 'got hello\n');
	h.p.resize(100, 30);
	await waitFor(h, 'resized 100x30\n');
	h.p.stdin.write(te.encode('q\n'));
	results.push('q=' + await h.p.exited + ' raws=' + h.raws.join(',') + ' after=' + h.p.stdin.write(te.encode('x')));

	// the end of its input
	h = start({ bytes: CHILD, argv: ['/nowhere/child'] });
	await waitFor(h, 'ready');
	h.p.stdin.write(te.encode('x\n'));
	h.p.stdin.close();
	results.push('eof=' + await h.p.exited + ' ' + JSON.stringify(h.text.split('\n').slice(1).join('|')));

	// killed while it waits on its stdin
	h = start({});
	await waitFor(h, 'ready');
	const k = h.p.kill();
	results.push('kill=' + k + ' code=' + await h.p.exited + ' again=' + h.p.kill() + ' tty=' + proc.tty(h.p.id) + ' resize=' + h.p.resize(1, 1));

	// no tty
	h = start({ tty: undefined });
	results.push('notty=' + await h.p.exited);

	// built by TinyGo, on a page whose loader is Go's: its own loader is
	// fetched for it, and the page keeps Go's
	if (TINY) {
		proc.assets.wasmExecTinyGo = TINY_EXEC;
		globalThis.importScripts = (u) => vm.runInThisContext(nodefs.readFileSync(u, 'utf8'));
		const pageGo = globalThis.Go;
		h = start({ bytes: TINY });
		await waitFor(h, 'ready 80x24\n');
		h.p.stdin.write(te.encode('tiny\n'));
		await waitFor(h, 'got tiny\n');
		h.p.resize(90, 20);
		await waitFor(h, 'resized 90x20\n');
		h.p.stdin.write(te.encode('q\n'));
		results.push('tinygo=' + await h.p.exited + ' raws=' + h.raws.join(',') + ' pageGo=' + (globalThis.Go === pageGo));
		delete globalThis.importScripts;
	}
	console.log(results.join('\n'));

	if (PARENT) {
		// The page's own program, run as a page runs it, with the guard a
		// page needs for a timer that fires after its main returns.
		const go = new Go();
		go.argv = ['parent'];
		const resume = go._resume.bind(go);
		go._resume = () => { if (!go.exited) { resume(); return; } for (const h of go._scheduledTimeouts.values()) clearTimeout(h); go._scheduledTimeouts.clear(); };
		await go.run(await WebAssembly.instantiate(PARENT, go.importObject).then((r) => r.instance));
	}
})().catch((e) => { console.log('FAIL ' + (e.stack || e)); });
`

func TestProcTTY(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	dir := t.TempDir()
	build := func(name string) string {
		out := filepath.Join(dir, name+".wasm")
		cmd := exec.Command("go", "build", "-o", out, "./testdata/"+name) //nolint:gosec // a fixed package of this repo
		cmd.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm")
		if b, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("build %s: %v\n%s", name, err, b)
		}
		return out
	}
	child, parent := build("ttychild"), build("ttyparent")
	var tiny, tinyExec string
	if tg, err := exec.LookPath("tinygo"); err == nil && !testing.Short() {
		tiny = filepath.Join(dir, "tiny.wasm")
		if b, err := exec.Command(tg, "build", "-o", tiny, "-target", "wasm", "-no-debug", "./testdata/ttychild").CombinedOutput(); err != nil { //nolint:gosec // a fixed package of this repo
			t.Fatalf("tinygo build: %v\n%s", err, b)
		}
		root, err := exec.Command(tg, "env", "TINYGOROOT").Output() //nolint:gosec // the toolchain found above
		if err != nil {
			t.Fatal(err)
		}
		tinyExec = filepath.Join(strings.TrimSpace(string(root)), "targets", "wasm_exec.js")
	}
	root, err := exec.Command("go", "env", "GOROOT").Output()
	if err != nil {
		t.Fatal(err)
	}
	wasmExec, err := os.ReadFile(filepath.Join(strings.TrimSpace(string(root)), "lib", "wasm", "wasm_exec.js")) //nolint:gosec // the toolchain's own loader
	if err != nil {
		t.Skip("no wasm_exec.js in this GOROOT:", err)
	}
	var s []byte
	for _, part := range [][]byte{[]byte(procTTYPrelude), JSFS(), wasmExec, ProcJS(), []byte(procTTYScript)} {
		s = append(append(s, part...), '\n')
	}
	script := filepath.Join(dir, "tty.js")
	if err := os.WriteFile(script, s, 0o600); err != nil { //nolint:gosec // a file in the test's temp dir
		t.Fatal(err)
	}
	cmd := exec.Command(node, script) //nolint:gosec // the script this test wrote
	cmd.Env = append(os.Environ(), "CHILD_WASM="+child, "PARENT_WASM="+parent, "TINY_WASM="+tiny, "TINY_EXEC="+tinyExec)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node: %v\n%s", err, out)
	}
	want := []string{
		`q=4 raws=true,false after=false`,
		`eof=5 "got x|eof|"`,
		`kill=true code=130 again=false tty=null resize=false`,
		`notty=2`,
	}
	if tiny != "" {
		want = append(want, `tinygo=4 raws=true,false pageGo=true`)
	}
	want = append(want,
		`run code=4 err=<nil> out="ready 40x10\ngot a\n" raws=[true false]`,
		`kill=true`,
		`killed code=130 err=<nil> write=io: read/write on closed pipe`,
	)
	// node's console.log ends each of the parent's writes with a newline of
	// its own.
	var lines []string
	for _, l := range strings.Split(string(out), "\n") {
		if l != "" {
			lines = append(lines, l)
		}
	}
	if got, want := strings.Join(lines, "\n"), strings.Join(want, "\n"); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}
