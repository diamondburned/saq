package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"

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
	includeDirs   []string = []string{"./..."}
	excludeDirs   []string = []string{".git"}
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	pflag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [flags...] argv...\n", os.Args[0])
		fmt.Fprintln(os.Stderr, "Flags:")
		pflag.PrintDefaults()
	}

	pflag.StringSliceVarP(&includeDirs, "include", "i", includeDirs, "include directory")
	pflag.StringSliceVarP(&excludeDirs, "exclude", "x", excludeDirs, "exclude directory")
	pflag.StringVarP(&sourceURL, "source", "s", sourceURL, "source URL of the upstream server")
	pflag.StringVarP(&targetAddr, "target", "t", targetAddr, "target address to listen on")
	pflag.StringVar(&gitignoreFile, "gitignore", gitignoreFile, "gitignore file to use, empty to disable")
	pflag.Parse()

	if pflag.NArg() == 0 {
		pflag.Usage()
		os.Exit(1)
	}

	src, err := url.Parse(sourceURL)
	if err != nil {
		log.Fatalln("invalid --source URL:", err)
	}

	wg, ctx := errgroup.WithContext(ctx)

	observer := NewObserver(Observed{
		Paths:     includeDirs,
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

	wg.Go(func() error {
		ch := observer.Subscribe()
		defer observer.Unsubscribe(ch)

		for {
			select {
			case <-ctx.Done():
				return nil
			case <-ch:
				runner.Restart()
			}
		}
	})

	r := http.NewServeMux()
	r.HandleFunc("/__refresh", func(w http.ResponseWriter, r *http.Request) {
		ch := runner.Subscribe()
		defer runner.Unsubscribe(ch)

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
