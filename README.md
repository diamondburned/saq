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

## How it works

`saq` acts as a reverse proxy. It works as follows:

1. It injects a script into the HTML response that `fetch`es an endpoint that
   blocks until a file change is detected.
2. When a file change is detected, the server process is restarted, which `saq`
   then tries to connect by sending a `HEAD` request to the server.
3. Once the server is up, the browser is reloaded.

## Supported Platforms

`saq` only works on Linux due to its dependency on [illarion/gonotify](https://github.com/illarion/gonotify).

## Who made the name?

[@FreezingSnail](https://github.com/freezingsnail)
