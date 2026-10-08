//go:build js && wasm

package proc

import (
	"os"
	"syscall/js"
)

// Terminal is a child's own terminal, when its parent started it with a TTY:
// the size to draw at, raw mode, and word of resizes. Its stdin and stdout are
// the terminal's keys and screen, as on any Unix.
type Terminal struct {
	v      js.Value
	resize []js.Func
}

// Term returns this program's terminal, or false when it has none: it was
// not spawned by proc, or was spawned without a TTY.
func Term() (*Terminal, bool) {
	proc := js.Global().Get("proc")
	id := os.Getenv("BOTTLE_PID")
	if !proc.Truthy() || !proc.Get("tty").Truthy() || id == "" {
		return nil, false
	}
	v := proc.Call("tty", id)
	if !v.Truthy() {
		return nil, false
	}
	return &Terminal{v: v}, true
}

// Size is the terminal's size in cells.
func (t *Terminal) Size() (cols, rows int) {
	s := t.v.Call("size")
	return s.Index(0).Int(), s.Index(1).Int()
}

// SetRaw turns raw mode on or off. In raw mode the parent passes every key
// through as typed, Ctrl+C among them; cooked, it echoes, edits a line and
// interrupts on Ctrl+C.
func (t *Terminal) SetRaw(on bool) {
	t.v.Call("setRaw", on)
}

// OnResize calls f, on a goroutine of its own, each time the terminal
// changes size.
func (t *Terminal) OnResize(f func(cols, rows int)) {
	jf := js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) >= 2 {
			go f(args[0].Int(), args[1].Int())
		}
		return nil
	})
	t.resize = append(t.resize, jf)
	t.v.Call("onResize", jf)
}
