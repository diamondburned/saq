package main

import (
	"context"
	"log"
	"net/http"
	"time"
)

type HTTPState int

const (
	HTTPStateUnknown HTTPState = iota
	HTTPStateAlive
	HTTPStateDead
)

func (s HTTPState) String() string {
	switch s {
	case HTTPStateUnknown:
		return "unknown"
	case HTTPStateAlive:
		return "alive"
	case HTTPStateDead:
		return "dead"
	default:
		return "invalid"
	}
}

type httpMonitorRefresh struct {
	until HTTPState
}

// HTTPMonitor is a HTTP monitor.
type HTTPMonitor struct {
	Subscriber[HTTPState]
	Addr string

	pubsub    *Pubsub[HTTPState]
	refresh   chan httpMonitorRefresh
	lastState HTTPState
}

// NewHTTPMonitor creates a new HTTP monitor.
func NewHTTPMonitor(addr string) *HTTPMonitor {
	pubsub := NewPubsub[HTTPState]()
	refresh := make(chan httpMonitorRefresh, 1)
	refresh <- httpMonitorRefresh{until: HTTPStateAlive}
	return &HTTPMonitor{
		Subscriber: pubsub,
		Addr:       addr,
		pubsub:     pubsub,
		refresh:    refresh,
	}
}

// Refresh refreshes the monitor. Refreshes may be debounced.
func (m *HTTPMonitor) Refresh() {
	select {
	case m.refresh <- httpMonitorRefresh{until: HTTPStateAlive}:
	default:
	}
}

// RefreshUntilState refreshes the monitor until the state is the given state.
// It blocks until the refresh is received.
func (m *HTTPMonitor) RefreshUntilState(ctx context.Context, until HTTPState) error {
	log.Println("delivering refresh until", until, "to http monitor")
	select {
	case <-ctx.Done():
		return ctx.Err()
	case m.refresh <- httpMonitorRefresh{until}:
		return nil
	}
}

func (m *HTTPMonitor) Start(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case refresh := <-m.refresh:
			log.Println("http monitor received refresh until", refresh.until)
			m.publish(HTTPStateUnknown)
			if err := m.pingHTTPUntilState(ctx, m.Addr, refresh.until); err != nil {
				return err
			}
		}
	}
}

func (m *HTTPMonitor) pingHTTPUntilState(ctx context.Context, addr string, until HTTPState) error {
	const retryDelay = 100 * time.Millisecond

	timer := time.NewTimer(retryDelay)
	defer timer.Stop()

	for {
		// We're still refreshing, so drain the refresh channel.
		select {
		case refresh := <-m.refresh:
			until = refresh.until
		default:
		}

		var state HTTPState

		r, err := http.Head(addr)
		if err == nil {
			r.Body.Close()

			log.Println("source server is alive")
			state = HTTPStateAlive
		} else {
			log.Println("cannot ping source server:", err)
			state = HTTPStateDead
		}

		m.publish(state)
		if state == until {
			log.Println("source server is now actually", state)
			return nil
		}

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

func (m *HTTPMonitor) publish(newState HTTPState) {
	if m.lastState != newState {
		m.lastState = newState
		m.pubsub.Publish(newState)
	}
}
