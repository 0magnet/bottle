//go:build js && wasm

// parent: spawns /bin/child with a piped stdin, collects its stdout, and
// reports what came back and the exit code — all inside one browser tab, two
// wasm instances, one shared filesystem.
package main

import (
	"bytes"
	"fmt"
	"strings"
	"syscall/js"

	"github.com/0magnet/bottle/proc"
)

func main() {
	out := js.Global().Get("document").Call("getElementById", "log")
	logln := func(s string) { out.Set("textContent", out.Get("textContent").String()+s+"\n") }

	var got bytes.Buffer
	c := proc.Command("/bin/child")
	c.Stdin = strings.NewReader("processes in a bottle\n")
	c.Stdout = &got
	c.Stderr = &got
	logln("parent: spawning /bin/child with piped stdin…")
	code, err := c.Run()
	if err != nil {
		logln("parent: spawn error: " + err.Error())
		return
	}
	logln(fmt.Sprintf("parent: child exited %d", code))
	logln("parent: child stdout = " + strings.TrimRight(got.String(), "\n"))
	if strings.TrimSpace(got.String()) == "PROCESSES IN A BOTTLE" && code == 3 {
		logln("PROC-MARKER: spawn + stdin + stdout + exit code all work")
	} else {
		logln("PROC-FAIL")
	}
	select {}
}
