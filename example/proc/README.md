# proc example — one wasm program spawns another

The parent program spawns `/bin/child` (a second wasm binary seeded into
jsfs), pipes it stdin, collects its stdout, and reads its exit code — two
wasm instances, one shared page filesystem, no server.

    # from this directory:
    GOOS=js GOARCH=wasm go build -o child.wasm ./child
    GOOS=js GOARCH=wasm go build -o parent.wasm ./parent
    cp ../../jsfs.js ../../proc.js .
    cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" .
    python3 -m http.server 8080   # open http://localhost:8080/

The page prints `PROC-MARKER: spawn + stdin + stdout + exit code all work`.
