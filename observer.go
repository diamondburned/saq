package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"strings"

	"github.com/illarion/gonotify/v2"
	gitignore "github.com/sabhiram/go-gitignore"
)

// Observed is a set of paths to observe.
type Observed struct {
	Root      string
	Excludes  []string
	Gitignore string
}

// Observer observes a set of paths for changes.
type Observer struct {
	Subscriber[struct{}]

	obs    Observed
	pubsub *Pubsub[struct{}]
}

// NewObserver creates a new observer for the given paths.
func NewObserver(observed Observed) *Observer {
	pubsub := NewPubsub[struct{}]()
	return &Observer{
		Subscriber: pubsub,
		obs:        observed,
		pubsub:     pubsub,
	}
}

const wmask = 0 |
	gonotify.IN_CREATE | gonotify.IN_DELETE | gonotify.IN_MODIFY |
	gonotify.IN_MOVED_FROM | gonotify.IN_MOVED_TO

// Start starts the observer until the context is canceled.
func (o *Observer) Start(ctx context.Context) error {
	watcher, err := gonotify.NewDirWatcher(ctx, wmask, o.obs.Root)
	if err != nil {
		return err
	}

	var ignore *gitignore.GitIgnore
	if o.obs.Gitignore != "" {
		ignore, err = gitignore.CompileIgnoreFile(o.obs.Gitignore)
		if err != nil {
			return fmt.Errorf("failed to compile .gitignore: %v", err)
		}
	}

eventLoop:
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case ev := <-watcher.C:
			if ev.Eof {
				return fmt.Errorf("watcher closed")
			}

			if ignore != nil && ignore.MatchesPath(ev.Name) {
				continue
			}

			for _, excl := range o.obs.Excludes {
				if first, rest := popFirstPart(excl); first == "." {
					if strings.HasPrefix(ev.Name, rest) {
						log.Printf("excluded %q on rule %q", ev, excl)
						continue eventLoop
					}
					continue
				}

				match, _ := filepath.Match(excl, ev.Name)
				if match {
					log.Printf("excluded %q on rule %q", ev, excl)
					continue eventLoop
				}
			}

			log.Println("file reloaded:", ev)
			o.pubsub.Publish(struct{}{})

		}
	}
}

func checkValidExclude(excl string) error {
	if first, rest := popFirstPart(excl); first == "." {
		if rest == "" {
			return errors.New("cannot exclude root")
		}
		return nil
	}

	if _, err := filepath.Match(excl, ""); err != nil {
		return fmt.Errorf("invalid exclude pattern %q: %w", excl, err)
	}

	return nil
}

func popFirstPart(path string) (first, rest string) {
	first, rest, ok := strings.Cut(path, string(filepath.Separator))
	if !ok {
		return path, ""
	}
	return first, rest
}
