package main

import (
	"context"
	"fmt"
	"log"

	"github.com/fsnotify/fsnotify"
	gitignore "github.com/sabhiram/go-gitignore"
)

// Observed is a set of paths to observe.
type Observed struct {
	Paths     []string
	Excludes  []string
	Gitignore string
}

// Observer observes a set of paths for changes.
type Observer struct {
	Subscriber[fsnotify.Event]

	obs    Observed
	pubsub *Pubsub[fsnotify.Event]
}

// NewObserver creates a new observer for the given paths.
func NewObserver(observed Observed) *Observer {
	pubsub := NewPubsub[fsnotify.Event]()
	return &Observer{
		Subscriber: pubsub,
		obs:        observed,
		pubsub:     pubsub,
	}
}

// Start starts the observer until the context is canceled.
func (o *Observer) Start(ctx context.Context) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	for _, path := range o.obs.Paths {
		if err := watcher.Add(path); err != nil {
			return fmt.Errorf("failed to watch path %q: %v", path, err)
		}
	}

	for _, path := range o.obs.Excludes {
		if err := watcher.Remove(path); err != nil {
			return fmt.Errorf("failed to exclude path %q: %v", path, err)
		}
	}

	var ignore *gitignore.GitIgnore
	if o.obs.Gitignore != "" {
		ignore, err = gitignore.CompileIgnoreFile(o.obs.Gitignore)
		if err != nil {
			return fmt.Errorf("failed to compile .gitignore: %v", err)
		}
	}

	for {
		select {
		case <-ctx.Done():
			if err := watcher.Close(); err != nil {
				return fmt.Errorf("failed to close watcher: %w", err)
			}
			return ctx.Err()
		case ev := <-watcher.Events:
			if ignore != nil && ignore.MatchesPath(ev.Name) {
				continue
			}
			log.Print("file reloaded: ", ev.Name, ": ", ev.Op)
			o.pubsub.Publish(ev)
		}
	}
}
