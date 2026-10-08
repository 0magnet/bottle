//go:build js && wasm

// ttyparent drives ttychild through the Go adapter: a run fed from a reader,
// and a child left waiting on its stdin until it is killed.
package main

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/0magnet/bottle/proc"
)

func main() {
	var out bytes.Buffer
	var raws []bool
	c := proc.Command("/bin/child")
	c.Stdin = strings.NewReader("a\nq\n")
	c.Stdout = &out
	c.TTY = &proc.TTY{Cols: 40, Rows: 10, OnRaw: func(on bool) { raws = append(raws, on) }}
	code, err := c.Run()
	fmt.Printf("run code=%d err=%v out=%q raws=%v\n", code, err, out.String(), raws)

	r, w := io.Pipe()
	defer w.Close()
	var out2 bytes.Buffer
	k := proc.Command("/bin/child")
	k.Stdin = r
	k.Stdout = &out2
	k.TTY = &proc.TTY{Cols: 80, Rows: 24}
	p, err := k.Start()
	if err != nil {
		fmt.Println("start:", err)
		return
	}
	for !strings.Contains(out2.String(), "ready") {
		time.Sleep(5 * time.Millisecond) // idle, so the page runs the child
	}
	fmt.Printf("kill=%v\n", p.Kill(false))
	code, err = p.Wait()
	fmt.Printf("killed code=%d err=%v\n", code, err)
}
