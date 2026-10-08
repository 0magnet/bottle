//go:build js && wasm

// ttyparent drives ttychild through the Go adapter: a run fed from a reader,
// and a child left waiting on its stdin until it is killed.
package main

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
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

	var out2 syncBuffer
	k := proc.Command("/bin/child")
	w, err := k.StdinPipe()
	if err != nil {
		fmt.Println("stdin pipe:", err)
		return
	}
	k.Stdout = &out2
	k.TTY = &proc.TTY{Cols: 80, Rows: 24}
	p, err := k.Start()
	if err != nil {
		fmt.Println("start:", err)
		return
	}
	until := func(s string) {
		for !strings.Contains(out2.String(), s) {
			time.Sleep(5 * time.Millisecond) // idle, so the page runs the child
		}
	}
	until("ready")
	w.Write([]byte("b\n")) //nolint:errcheck,gosec // checked by what the child echoes
	until("got b")
	fmt.Printf("kill=%v\n", p.Kill(false))
	code, err = p.Wait()
	_, werr := w.Write([]byte("c\n"))
	fmt.Printf("killed code=%d err=%v write=%v\n", code, err, werr)
}

// syncBuffer is read by main while the process's writer goroutine fills it.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}
