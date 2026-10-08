//go:build js && wasm

// Package proc is the Go adapter for bottle's process layer (proc.js): spawn
// another wasm program from the page filesystem as a child that shares this
// tab's fs and vnet, wire its stdio, and wait for it to exit.
//
// It is deliberately small and explicit rather than a drop-in for os/exec:
// os/exec on js/wasm fails in syscall.StartProcess (ENOSYS), and making the
// standard package work needs a patched GOROOT. Cmd here is the honest tab
// primitive that a shell — or, with the GOROOT overlay, cmd/go — builds on.
package proc

import (
	"errors"
	"io"
	"strings"
	"syscall/js"
)

// Cmd is one child process: a program in the page filesystem plus the argv,
// env, cwd and stdio to run it with. Zero value is not useful; use Command.
type Cmd struct {
	Path   string   // argv[0]: resolved against jsfs (abs, cwd-relative, or PATH)
	Args   []string // argv, including Args[0] == the program name
	Env    []string // "KEY=value"; empty inherits nothing (pass explicitly)
	Dir    string   // working directory; empty means the page cwd
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer

	// TTY, if set, gives the child a terminal; see TTY.
	TTY *TTY

	// OffThread runs the child in a Worker instead of on the page's one JS
	// thread, so a long build does not freeze the tab. The child still sees
	// this tab's filesystem -- it reaches jsfs over a blocking channel -- but
	// not its vnet, and its stdin reads EOF. Suitable for compute like
	// cmd/compile and cmd/link; not for a child that listens or dials.
	//
	// It needs cross-origin isolation, which is a property of how the page was
	// served (COOP/COEP headers). Where that is missing, Run falls back to an
	// on-thread child: same result, same output, just a busy main thread. Use
	// OffThreadAvailable to find out which one you will get.
	OffThread bool
}

// OffThreadAvailable reports whether OffThread children can actually run here:
// proc.js and fsbridge.js are loaded and the page is cross-origin isolated.
func OffThreadAvailable() bool {
	proc := js.Global().Get("proc")
	if !proc.Truthy() || !proc.Get("spawnWorker").Truthy() {
		return false
	}
	if !js.Global().Get("fsbridge").Truthy() {
		return false
	}
	return js.Global().Get("crossOriginIsolated").Truthy()
}

// Command builds a Cmd, mirroring os/exec.Command's shape.
func Command(name string, arg ...string) *Cmd {
	return &Cmd{Path: name, Args: append([]string{name}, arg...)}
}

// TTY makes a child a terminal program: it starts at Cols×Rows, finds its
// size and sets raw mode through Terminal, and hears of Process.Resize.
type TTY struct {
	Cols, Rows int
	// OnRaw, if set, is told each time the child turns raw mode on or off,
	// so the parent's terminal can stop (or resume) cooking its input.
	OnRaw func(raw bool)
}

// Process is a started child.
type Process struct {
	ID  string // the id the child was handed in BOTTLE_PID
	Pid int

	v     js.Value
	funcs []js.Func
	done  chan struct{}
	code  int
	err   error
}

// Run spawns the child and blocks until it exits, returning its exit code and
// any spawn error. Stdout/Stderr receive the child's output; Stdin, if set,
// feeds its input.
func (c *Cmd) Run() (int, error) {
	p, err := c.Start()
	if err != nil {
		return -1, err
	}
	return p.Wait()
}

// Start spawns the child and returns without waiting for it.
//
// Its output reaches Stdout and Stderr as it is written, each write in a
// callback of its own: a writer there must not block, or the page stalls
// with it. Stdin is copied into the child's stdin pipe by a goroutine, and
// a child reading it waits as one reading a terminal does.
func (c *Cmd) Start() (*Process, error) {
	proc := js.Global().Get("proc")
	if !proc.Truthy() {
		return nil, errors.New("proc: proc.js not loaded on this page")
	}
	p := &Process{done: make(chan struct{})}
	fn := func(f func(_ js.Value, args []js.Value) any) js.Func {
		jf := js.FuncOf(f)
		p.funcs = append(p.funcs, jf)
		return jf
	}

	opts := js.Global().Get("Object").New()
	argv := js.Global().Get("Array").New()
	for _, a := range c.Args {
		argv.Call("push", a)
	}
	opts.Set("argv", argv)
	if c.Dir != "" {
		opts.Set("cwd", c.Dir)
	}
	env := js.Global().Get("Object").New()
	for _, kv := range c.Env {
		if k, v, ok := strings.Cut(kv, "="); ok {
			env.Set(k, v)
		}
	}
	opts.Set("env", env)

	if c.Stdout != nil {
		opts.Set("stdout", fn(sink(c.Stdout)))
	}
	if c.Stderr != nil {
		opts.Set("stderr", fn(sink(c.Stderr)))
	}

	method := "spawn"
	if c.OffThread && OffThreadAvailable() {
		method = "spawnWorker" // a worker child's stdin reads EOF, and it has no terminal
	} else {
		if c.Stdin != nil {
			opts.Set("stdin", "pipe")
		}
		if c.TTY != nil {
			t := js.Global().Get("Object").New()
			t.Set("cols", c.TTY.Cols)
			t.Set("rows", c.TTY.Rows)
			if on := c.TTY.OnRaw; on != nil {
				t.Set("onRaw", fn(func(_ js.Value, args []js.Value) any {
					on(len(args) > 0 && args[0].Truthy())
					return nil
				}))
			}
			opts.Set("tty", t)
		}
	}

	p.v = proc.Call(method, opts)
	p.ID = p.v.Get("id").String()
	p.Pid = p.v.Get("pid").Int()
	if c.Stdin != nil && method == "spawn" {
		go feed(c.Stdin, p.v.Get("stdin"))
	}
	go func() {
		code, err := await(p.v.Get("exited"))
		if err != nil {
			p.code, p.err = -1, err
		} else {
			p.code = code.Int()
		}
		close(p.done)
	}()
	return p, nil
}

// Wait blocks until the child exits and returns its exit code.
func (p *Process) Wait() (int, error) {
	<-p.done
	// Output already written was queued on microtasks ahead of the exit, so
	// the sinks have run; nothing calls them now.
	for _, f := range p.funcs {
		f.Release()
	}
	p.funcs = nil
	return p.code, p.err
}

// Kill interrupts the child through its own handler when it registered one,
// and otherwise stops it outright, as kill -9 would; it then exits 130.
// Force skips the handler. It reports whether there was anything to kill.
func (p *Process) Kill(force bool) bool {
	return p.v.Call("kill", force).Truthy()
}

// Resize tells a child started with a TTY that its terminal changed size.
func (p *Process) Resize(cols, rows int) {
	p.v.Call("resize", cols, rows)
}

// feed copies r into a child's stdin pipe until r ends or the child goes.
func feed(r io.Reader, stdin js.Value) {
	defer stdin.Call("close")
	buf := make([]byte, 32*1024)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			u := js.Global().Get("Uint8Array").New(n)
			js.CopyBytesToJS(u, buf[:n])
			if !stdin.Call("write", u).Truthy() {
				return
			}
		}
		if err != nil {
			return
		}
	}
}

// sink adapts an io.Writer to proc.js's stdout/stderr callback, which is
// called with a Uint8Array chunk per write.
func sink(w io.Writer) func(js.Value, []js.Value) any {
	return func(_ js.Value, args []js.Value) any {
		if len(args) == 0 {
			return nil
		}
		b := make([]byte, args[0].Get("length").Int())
		js.CopyBytesToGo(b, args[0])
		w.Write(b) //nolint:errcheck,gosec // a sink that cannot accept output is the caller's problem, not the child's
		return nil
	}
}

// await blocks a goroutine on a JS promise, returning its resolved value or a
// rejection as an error.
func await(promise js.Value) (js.Value, error) {
	type result struct {
		v   js.Value
		err error
	}
	ch := make(chan result, 1)
	then := js.FuncOf(func(_ js.Value, args []js.Value) any {
		var v js.Value
		if len(args) > 0 {
			v = args[0]
		}
		ch <- result{v: v}
		return nil
	})
	defer then.Release()
	catch := js.FuncOf(func(_ js.Value, args []js.Value) any {
		msg := "promise rejected"
		if len(args) > 0 {
			msg = args[0].Call("toString").String()
		}
		ch <- result{err: errors.New(msg)}
		return nil
	})
	defer catch.Release()
	promise.Call("then", then).Call("catch", catch)
	r := <-ch
	return r.v, r.err
}
