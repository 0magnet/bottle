//go:build js && wasm

// ttychild is a terminal program run under proc: it reports its size, goes
// raw, echoes each line it reads, and reports resizes. "q" exits 4; the end
// of its input exits 5.
package main

import (
	"bufio"
	"fmt"
	"os"

	"github.com/0magnet/bottle/proc"
)

func main() {
	t, ok := proc.Term()
	if !ok {
		fmt.Println("no tty")
		os.Exit(2)
	}
	t.OnResize(func(c, r int) { fmt.Printf("resized %dx%d\n", c, r) })
	t.SetRaw(true)
	c, r := t.Size()
	fmt.Printf("ready %dx%d\n", c, r)
	sc := bufio.NewScanner(os.Stdin)
	for sc.Scan() {
		if sc.Text() == "q" {
			t.SetRaw(false)
			os.Exit(4)
		}
		fmt.Printf("got %s\n", sc.Text())
	}
	fmt.Println("eof")
	os.Exit(5)
}
