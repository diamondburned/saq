# saq

A toolchain-agnostic live reload tool that supports automatic reloading of the
browser when a file is changed.

## Basic Example

```sh
saq \
    -s http://localhost:8081 \
    -t localhost:8080 \
    -i ./... \
    -x .git \
    --gitignoreFile .gitignore \
    -- bash -c 'make build && ./build/main --http localhost:8081'
# or equivalently
saq -- bash -c 'make build && ./build/main --http localhost:8081'
```

**Note**: `go run` does not propagate signals to the child process, so you
should not use `go run` with `saq`. See https://github.com/golang/go/issues/40467.

## Unimplemented Features

Right now, `saq` doesn't detect if the server is ready to accept connections,
which means the browser may reload before the server is ready.

This should be easily fixable with something like a `HEAD` request to the server
to check if it's ready. For now, it's unimplemented.

## Who made the name?

@FreezeSnail
