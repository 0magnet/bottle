//go:build js && wasm

// child: reads all of stdin, writes it back upper-cased, exits 3. Proof that
// a spawned wasm program has working stdin, stdout and a real exit code.
package main

import (
	"fmt"
	"io"
	"os"
	"strings"
)

func main() {
	b, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintln(os.Stderr, "child: read stdin:", err)
		os.Exit(1)
	}
	if _, err := os.Stdout.Write([]byte(strings.ToUpper(string(b)))); err != nil {
		fmt.Fprintln(os.Stderr, "child: write stdout:", err)
		os.Exit(1)
	}
	// 3, not 0: the parent asserts on the exit code, so it has to be one that
	// could not have come from a success path.
	os.Exit(3)
}
