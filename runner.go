package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// Runner is a runner.
type Runner interface {
	Subscriber[struct{}]
	// Restart signals the runner to restart.
	// After the runner has restarted the command, a signal should be emitted to
	// the Subscriber instance.
	Restart()
}

// NoopRunner is a no-op runner. It doesn't run any command but can fully
// emulate the behavior of a forever-blocking command.
type NoopRunner struct {
	Subscriber[struct{}]
	pubsub *Pubsub[struct{}]
}

// NewNoopRunner creates a new no-op runner.
func NewNoopRunner() *NoopRunner {
	pubsub := NewPubsub[struct{}]()
	return &NoopRunner{
		Subscriber: pubsub,
		pubsub:     pubsub,
	}
}

// Restart signals the no-op runner to restart.
func (r *NoopRunner) Restart() {
	go r.pubsub.Publish(struct{}{})
}

// CommandRunner is a command runner. It maintains a running command in the
// background.
type CommandRunner struct {
	Subscriber[struct{}]

	args    []string
	restart chan struct{}
	pubsub  *Pubsub[struct{}]
}

// NewCommandRunner creates a new command runner.
func NewCommandRunner(args []string) *CommandRunner {
	restart := make(chan struct{}, 1)
	restart <- struct{}{}

	pubsub := NewPubsub[struct{}]()
	return &CommandRunner{
		pubsub,
		args,
		restart,
		pubsub,
	}
}

// Start starts the command runner until the context is canceled.
func (s *CommandRunner) Start(ctx context.Context) error {
	var cmd *exec.Cmd
	defer func() {
		if cmd != nil {
			stopCommand(cmd)
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.restart:
		}

		log.Println("command runner received restart")

		if cmd != nil {
			stopCommand(cmd)
			cmd = nil
		}

		log.Printf("starting command %q", s.args)

		cmd = exec.Command(s.args[0], s.args[1:]...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

		if err := cmd.Start(); err != nil {
			return fmt.Errorf("failed to start process: %w", err)
		}

		// sleep for a bit to wait for the process to start
		if err := sleep(ctx, 500*time.Millisecond); err != nil {
			return err
		}

		// drain restart channel
		select {
		case <-s.restart:
		default:
		}

		s.pubsub.Publish(struct{}{})
	}
}

// Restart signals the command runner to restart the command.
func (s *CommandRunner) Restart() {
	select {
	case s.restart <- struct{}{}:
	default:
	}
}

func stopCommand(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}

	wait := make(chan error, 1)
	go func() { wait <- cmd.Wait() }()

	select {
	case <-wait:
		return
	default:
		syscall.Kill(-cmd.Process.Pid, syscall.SIGINT)
		log.Println("sent SIGINT, waiting 2s")
	}

	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()

	select {
	case <-timer.C:
		log.Println("timeout waiting for process to exit, killing...")
		syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	case <-wait:
		log.Println("process exited")
		return
	}

	if err := <-wait; err != nil {
		log.Println("error waiting for process to exit:", err)
	} else {
		log.Println("process exited")
	}
}

func sleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
