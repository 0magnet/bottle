//go:build js && wasm

// ttychild is a terminal program run under proc: it reports its size, goes
// raw, echoes each line it reads, and reports resizes. "q" exits 4; the end
// of its input exits 5. Built by Go it uses os.Stdin and os.Stdout; built by
// TinyGo, whose os.Stdin cannot read, the terminal itself.
package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"runtime"

	"github.com/0magnet/bottle/proc"
)

func main() {
	t, ok := proc.Term()
	if !ok {
		fmt.Println("no tty")
		os.Exit(2)
	}
	var in io.Reader = os.Stdin
	var out io.Writer = os.Stdout
	if runtime.Compiler == "tinygo" {
		in, out = t, t
	}
	t.OnResize(func(c, r int) { fmt.Fprintf(out, "resized %dx%d\n", c, r) })
	t.SetRaw(true)
	c, r := t.Size()
	fmt.Fprintf(out, "ready %dx%d %s\n", c, r, proc.Getenv("GREETING"))
	sc := bufio.NewScanner(in)
	for sc.Scan() {
		if sc.Text() == "q" {
			t.SetRaw(false)
			os.Exit(4)
		}
		fmt.Fprintf(out, "got %s\n", sc.Text())
	}
	fmt.Fprintln(out, "eof")
	os.Exit(5)
}
