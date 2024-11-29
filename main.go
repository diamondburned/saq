package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"

	"github.com/pkg/browser"
	"github.com/spf13/pflag"
	"golang.org/x/sync/errgroup"
	"libdb.so/hserve"
	"libdb.so/saq/internal/atomicg"
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
	sourceURL        = "http://localhost:8081"
	targetAddr       = "localhost:8080"
	fileServerAddr   = ""
	gitignoreFile    = ".gitignore"
	includeDir       = "."
	excludeDirs      = []string{"*.tmpl", "./vendor"}
	generateCheckCmd = `[[ $FILE == *.go ]] && grep "^// Code generated by" "$FILE"`
	noBrowser        = false
	browserOpenOnce  = true
	verbose          = false
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
	pflag.StringSliceVarP(&excludeDirs, "exclude", "x", excludeDirs, "exclude directories/paths/globs (prefix ./ is required for path)")
	pflag.StringVarP(&sourceURL, "source", "s", sourceURL, "source URL of the upstream server")
	pflag.StringVarP(&targetAddr, "target", "t", targetAddr, "target address to listen on")
	pflag.StringVarP(&fileServerAddr, "file-server", "F", fileServerAddr, "file server address to listen on, empty to disable")
	pflag.StringVar(&gitignoreFile, "gitignore", gitignoreFile, "gitignore file to use, empty to disable")
	pflag.StringVar(&generateCheckCmd, "generated-check", generateCheckCmd, "command to check if a file is generated, executes $SHELL or /bin/sh otherwise")
	pflag.BoolVar(&noBrowser, "no-browser", noBrowser, "do not open browser")
	pflag.BoolVar(&browserOpenOnce, "browser-open-once", browserOpenOnce, "only open browser once, otherwise it will open if there are no active browsers")
	pflag.BoolVarP(&verbose, "verbose", "v", verbose, "verbose logging")
	pflag.Parse()

	if len(os.Args) < 2 {
		pflag.Usage()
		os.Exit(1)
	}

	// These are always excluded.
	excludeDirs = append(excludeDirs, "./.git", "./.direnv")

	for _, excl := range excludeDirs {
		if err := checkValidExclude(excl); err != nil {
			log.Fatalln("invalid --exclude:", err)
		}
	}

	if fileServerAddr != "" && sourceURL != "" {
		log.Println("warning: --file-server is enabled, --source will be ignored")
		sourceURL = fileServerAddr
	}

	if !strings.Contains(sourceURL, "://") {
		sourceURL = "http://" + sourceURL
	}

	src, err := url.Parse(sourceURL)
	if err != nil {
		log.Fatalln("invalid --source URL:", err)
	}

	if !verbose {
		log.SetOutput(io.Discard)
	}

	var browserCount atomicg.Int

	wg, ctx := errgroup.WithContext(ctx)

	observer := NewObserver(Observed{
		Root:              includeDir,
		Excludes:          excludeDirs,
		Gitignore:         gitignoreFile,
		GeneratedCheckCmd: generateCheckCmd,
	})
	wg.Go(func() error {
		return observer.Start(ctx)
	})

	var runner Runner
	if len(pflag.Args()) == 0 {
		runner = NewNoopRunner()
	} else {
		cmdRunner := NewCommandRunner(pflag.Args())
		wg.Go(func() error {
			return cmdRunner.Start(ctx)
		})
		runner = cmdRunner
	}

	if fileServerAddr != "" {
		wg.Go(func() error {
			fs := http.FileServer(http.Dir(includeDir))
			log.Println("file server is listening on", fileServerAddr)
			return hserve.ListenAndServe(ctx, fileServerAddr, fs)
		})
	}

	serverMon := NewHTTPMonitor(sourceURL)
	wg.Go(func() error {
		return serverMon.Start(ctx)
	})

	wg.Go(func() error {
		observeCh := observer.Subscribe()
		defer observer.Unsubscribe(observeCh)

		runnerCh := runner.Subscribe()
		defer runner.Unsubscribe(runnerCh)

		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-observeCh:
				log.Println("observer detected changes, restarting runner")
				runner.Restart()
			case <-runnerCh:
				log.Println("runner restarted, monitoring server until it's alive")
				serverMon.RefreshUntilState(ctx, HTTPStateAlive)
			}
		}
	})

	wg.Go(func() error {
		if noBrowser {
			return nil
		}

		ch := serverMon.Subscribe()
		defer serverMon.Unsubscribe(ch)

		for {
			select {
			case <-ctx.Done():
				return ctx.Err()

			case state := <-ch:
				if state != HTTPStateAlive {
					continue
				}
			}

			// Open a browser if there are no other browsers open.
			if browserCount.Get() == 0 {
				addr := targetAddr
				if !strings.Contains(addr, "://") {
					addr = "http://" + addr
				}

				if err := browser.OpenURL(addr); err != nil {
					log.Println("error opening browser:", err)
				}
			}
		}
	})

	r := http.NewServeMux()
	r.HandleFunc("/__refresh", func(w http.ResponseWriter, r *http.Request) {
		ch := serverMon.Subscribe()
		defer serverMon.Unsubscribe(ch)

		if browserOpenOnce {
			browserCount.Set(1)
		} else {
			browserCount.Add(1)
			defer browserCount.Add(-1)
		}

		for {
			select {
			case <-r.Context().Done():
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			case state := <-ch:
				if state == HTTPStateAlive {
					log.Println("server is alive, refreshing page")
					w.WriteHeader(http.StatusNoContent)
					return
				}
			}
		}
	})

	r.Handle("/", proxy.NewReverseProxy(*src, func(body []byte) []byte {
		return append(body, []byte(hook)...)
	}))

	wg.Go(func() error {
		log.Println("listening on", targetAddr)
		return hserve.ListenAndServe(ctx, targetAddr, r)
	})

	if err := wg.Wait(); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatalln("error:", err)
	}
}

func assert(cond bool, msg string) {
	if !cond {
		log.Fatalln(msg)
	}
}
