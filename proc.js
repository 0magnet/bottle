// proc.js — the third leg of bottle: processes for wasm tabs.
//
// jsfs fakes the filesystem and vnet fakes the network; a Unix-shaped
// orchestrator — a shell, `go build`, make — also needs fork/exec. A tab has
// an exact analogue: instantiating another wasm module IS spawning a process.
// proc makes that a primitive.
//
//   globalThis.proc.spawn({argv, env, cwd, stdout, stderr, stdin})
//     -> { pid, exited: Promise<exitCode> }
//
// - argv[0] is resolved against jsfs (absolute, cwd-relative, or PATH-walked);
//   the file's bytes ARE the program. Compiled modules are cached by path so
//   repeat spawns skip the compile.
// - The child shares globalThis.fs (jsfs) and globalThis.vnet — that sharing
//   is the whole point: a parent writes $WORK, the child compiler reads it.
// - stdio is per-process. fds 1/2 route to the caller's stdout/stderr sinks
//   and fd 0 pulls from stdin; unset streams inherit the page defaults. The
//   active set is swapped around each wasm execution slice (each _resume is
//   synchronous and atomic on the one JS thread), so interleaved processes
//   never cross streams. Pipe them together with proc.pipe().
// - wait is the child's exit promise; the Go runtime's wasmExit resolves it.
//
// Requires jsfs.js (globalThis.fs, jsfs.stdio) and wasm_exec.js (globalThis.Go)
// loaded first. Load proc.js after both.
(function () {
	if (globalThis.proc && globalThis.proc.installed) return;
	if (!globalThis.fs || !globalThis.jsfs) throw new Error("proc.js: load jsfs.js first");
	if (typeof globalThis.Go !== "function") throw new Error("proc.js: load wasm_exec.js first");

	const jsfs = globalThis.jsfs;
	const stdio = jsfs.stdio; // { stdout, stderr, stdin } — the page defaults

	// The stdio set of the process whose execution slice is currently running.
	// jsfs.stdio's methods delegate here, so a child's fd writes reach its own
	// sinks without any per-call fd bookkeeping in jsfs.
	let active = null;
	const pageDefaults = { stdout: stdio.stdout, stderr: stdio.stderr, stdin: stdio.stdin };
	stdio.stdout = (b) => (active ? active.stdout : pageDefaults.stdout)(b);
	stdio.stderr = (b) => (active ? active.stderr : pageDefaults.stderr)(b);
	stdio.stdin = () => (active ? active.stdin : pageDefaults.stdin)();

	const moduleCache = new Map(); // path -> WebAssembly.Module

	function readProgram(argv0, cwd, env) {
		// Resolve argv[0] to bytes in jsfs. Absolute or cwd-relative first,
		// then a PATH walk (env.PATH, colon-separated) as a shell would.
		const tryPath = (p) => {
			const bytes = jsfs.readFile(p);
			return bytes ? { path: p, bytes } : null;
		};
		if (argv0.startsWith("/")) return tryPath(argv0);
		if (argv0.includes("/")) return tryPath(join(cwd || jsfs.getCwd(), argv0));
		for (const dir of ((env && env.PATH) || "/bin").split(":")) {
			const hit = tryPath(join(dir, argv0));
			if (hit) return hit;
		}
		return null;
	}

	function join(a, b) {
		if (b.startsWith("/")) return b;
		return (a.endsWith("/") ? a : a + "/") + b;
	}

	let nextPID = 2; // 1 is the page's own root program by convention

	function spawn(opts) {
		opts = opts || {};
		const argv = opts.argv || [];
		if (!argv.length) throw new Error("proc.spawn: empty argv");
		const cwd = opts.cwd || jsfs.getCwd();
		const env = opts.env || {};

		const prog = readProgram(argv[0], cwd, env);
		const pid = nextPID++;
		if (!prog) {
			// No such file: a real ENOENT, surfaced as a nonzero exit so a
			// shell prints "not found" rather than hanging.
			(opts.stderr || pageDefaults.stderr)(new TextEncoder().encode(argv[0] + ": not found\n"));
			return { pid, exited: Promise.resolve(127) };
		}

		const myStdio = {
			stdout: opts.stdout || pageDefaults.stdout,
			stderr: opts.stderr || pageDefaults.stderr,
			stdin: opts.stdin || pageDefaults.stdin,
		};

		const go = new Go();
		go.argv = argv.slice();
		go.env = Object.assign({}, env);
		let exitCode = 0;
		go.exit = (c) => { exitCode = c; };

		// Wrap _resume so this process's stdio is the active set for the exact
		// span of each synchronous execution slice, then restored. Covers the
		// initial run and every timer/promise-driven re-entry.
		const rawResume = go._resume.bind(go);
		go._resume = function () {
			const prev = active;
			active = myStdio;
			const prevCwd = jsfs.getCwd();
			jsfs.setCwd(cwd); // each process has its own working directory
			try {
				return rawResume();
			} finally {
				active = prev;
				jsfs.setCwd(prevCwd);
			}
		};

		const exited = (async () => {
			let mod = moduleCache.get(prog.path);
			if (!mod) {
				mod = await WebAssembly.compile(prog.bytes);
				moduleCache.set(prog.path, mod);
			}
			const inst = await WebAssembly.instantiate(mod, go.importObject);
			const prev = active;
			active = myStdio;
			const prevCwd = jsfs.getCwd();
			jsfs.setCwd(cwd);
			try {
				await go.run(inst); // resolves when the program exits
			} finally {
				active = prev;
				jsfs.setCwd(prevCwd);
			}
			return exitCode;
		})();

		return { pid, exited };
	}

	// pipe() returns an in-memory byte pipe: { write, close, reader } where
	// write/close feed a queue and reader() is a stdin-shaped puller (returns
	// a Uint8Array chunk, or null at EOF). Connect one process's stdout to the
	// next's stdin to build a shell pipeline.
	function pipe() {
		const chunks = [];
		let closed = false;
		return {
			write: (b) => { chunks.push(b); },
			close: () => { closed = true; },
			reader: () => (chunks.length ? chunks.shift() : (closed ? null : new Uint8Array(0))),
		};
	}

	globalThis.proc = { installed: true, spawn, pipe };
})();
