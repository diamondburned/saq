package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"time"

	"github.com/spf13/pflag"
	"golang.org/x/sync/errgroup"
	"libdb.so/hserve"
	"libdb.so/saq/internal/proxy"
)

// hook is the script that is injected into the page to wait for the refresh
// signal.
const hook = `
	<script>
		fetch("/__refresh").then(() => window.location.reload());
	</script>
`

var (
	sourceURL     string   = "http://localhost:8081"
	targetAddr    string   = "localhost:8080"
	gitignoreFile string   = ".gitignore"
	includeDir    string   = "."
	excludeDirs   []string = []string{"./.git", "./.direnv", "*.tmpl"}
	verbose       bool     = false
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	pflag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [flags...] argv...\n", os.Args[0])
		fmt.Fprintln(os.Stderr, "Flags:")
		pflag.PrintDefaults()
	}

	pflag.StringVarP(&includeDir, "include", "i", includeDir, "include directory")
	pflag.StringSliceVarP(&excludeDirs, "exclude", "x", excludeDirs, "exclude directories/paths/globs")
	pflag.StringVarP(&sourceURL, "source", "s", sourceURL, "source URL of the upstream server")
	pflag.StringVarP(&targetAddr, "target", "t", targetAddr, "target address to listen on")
	pflag.StringVar(&gitignoreFile, "gitignore", gitignoreFile, "gitignore file to use, empty to disable")
	pflag.BoolVarP(&verbose, "verbose", "v", verbose, "verbose logging")
	pflag.Parse()

	if pflag.NArg() == 0 {
		pflag.Usage()
		os.Exit(1)
	}

	for _, excl := range excludeDirs {
		if err := checkValidExclude(excl); err != nil {
			log.Fatalln("invalid --exclude:", err)
		}
	}

	src, err := url.Parse(sourceURL)
	if err != nil {
		log.Fatalln("invalid --source URL:", err)
	}

	if !verbose {
		log.SetOutput(io.Discard)
	}

	wg, ctx := errgroup.WithContext(ctx)

	observer := NewObserver(Observed{
		Root:      includeDir,
		Excludes:  excludeDirs,
		Gitignore: gitignoreFile,
	})
	wg.Go(func() error {
		return observer.Start(ctx)
	})

	runner := NewCommandRunner(pflag.Args())
	wg.Go(func() error {
		return runner.Start(ctx)
	})

	serverAlive := NewPubsub[struct{}]()

	wg.Go(func() error {
		ch := observer.Subscribe()
		defer observer.Unsubscribe(ch)

		for {
			select {
			case <-ctx.Done():
				return nil
			case <-ch:
				runner.Restart()

				if err := pingHTTPUntilAlive(ctx, sourceURL); err != nil {
					log.Println("cannot ping source server:", err)
					continue
				}

				log.Println("signaling server to refresh")
				serverAlive.Publish(struct{}{})
			}
		}
	})

	r := http.NewServeMux()
	r.HandleFunc("/__refresh", func(w http.ResponseWriter, r *http.Request) {
		ch := serverAlive.Subscribe()
		defer serverAlive.Unsubscribe(ch)

		select {
		case <-r.Context().Done():
			return
		case <-ch:
			w.WriteHeader(http.StatusNoContent)
		}
	})

	r.Handle("/", proxy.NewReverseProxy(*src, func(body []byte) []byte {
		return append(body, []byte(hook)...)
	}))

	wg.Go(func() error {
		log.Println("listening on", targetAddr)
		return hserve.ListenAndServe(ctx, targetAddr, r)
	})

	if err := wg.Wait(); err != nil {
		log.Fatalln("error:", err)
	}
}

func assert(cond bool, msg string) {
	if !cond {
		log.Fatalln(msg)
	}
}

func pingHTTPUntilAlive(ctx context.Context, addr string) error {
	const retryDelay = 100 * time.Millisecond

	timer := time.NewTimer(retryDelay)
	defer timer.Stop()

	for {
		r, err := http.Head(sourceURL)
		if err == nil {
			r.Body.Close()
			log.Println("source server is alive")
			return nil
		}

		log.Println("cannot ping source server:", err)
		log.Println("retrying in", retryDelay)

		timer.Reset(retryDelay)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			// ok
		}
	}
}
