//go:build js && wasm

// child: reads all of stdin, writes it back upper-cased, exits 3. Proof that
// a spawned wasm program has working stdin, stdout and a real exit code.
package main

import (
	"io"
	"os"
	"strings"
)

func main() {
	b, _ := io.ReadAll(os.Stdin)
	os.Stdout.Write([]byte(strings.ToUpper(string(b))))
	os.Exit(3)
}
