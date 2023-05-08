package main

import (
	"sync"
)

type Subscriber[T any] interface {
	Subscribe() <-chan T
	Unsubscribe(ch <-chan T)
}

// Pubsub is a pubsub.
type Pubsub[T any] struct {
	subs sync.Map // map[<-chan T]chan T
}

// NewPubsub creates a new pubsub.
func NewPubsub[T any]() *Pubsub[T] {
	return &Pubsub[T]{
		subs: sync.Map{},
	}
}

// Subscribe subscribes to the pubsub.
func (p *Pubsub[T]) Subscribe() <-chan T {
	ch := make(chan T)
	p.subs.Store((<-chan T)(ch), ch)
	return ch
}

// Unsubscribe unsubscribes from the pubsub.
func (p *Pubsub[T]) Unsubscribe(ch <-chan T) {
	p.subs.Delete((<-chan T)(ch))
}

// Publish publishes to the pubsub.
func (p *Pubsub[T]) Publish(value T) {
	p.subs.Range(func(k, v interface{}) bool {
		ch := v.(chan T)
		select {
		case ch <- value:
		default:
		}
		return true
	})
}
