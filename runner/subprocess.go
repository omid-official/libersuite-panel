package runner

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

const (
	minRestartDelay = 2 * time.Second
	maxRestartDelay = 60 * time.Second
)

type Process struct {
	Name string
	Bin  string
	Args []string
}

func (p *Process) Run(ctx context.Context) {
	delay := minRestartDelay
	for {
		if ctx.Err() != nil {
			return
		}
		log.Printf("[%s] starting", p.Name)
		start := time.Now()
		err := p.runOnce(ctx)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			log.Printf("[%s] exited with error: %v — restarting in %s", p.Name, err, delay)
		} else {
			log.Printf("[%s] exited unexpectedly — restarting in %s", p.Name, delay)
		}
		if time.Since(start) > 10*time.Second {
			delay = minRestartDelay
		}
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return
		}
		delay = min(delay*2, maxRestartDelay)
	}
}

func (p *Process) runOnce(ctx context.Context) error {
	cmd := exec.Command(p.Bin, p.Args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start: %w", err)
	}

	var wg sync.WaitGroup
	wg.Add(2)

	forward := func(r io.Reader) {
		defer wg.Done()
		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			log.Printf("[%s] %s", p.Name, scanner.Text())
		}
	}
	go forward(stdout)
	go forward(stderr)

	waitCh := make(chan error, 1)
	go func() {
		wg.Wait()
		waitCh <- cmd.Wait()
	}()

	select {
	case err := <-waitCh:
		return err
	case <-ctx.Done():
		_ = cmd.Process.Signal(syscall.SIGTERM)
		select {
		case <-waitCh:
		case <-time.After(5 * time.Second):
			_ = cmd.Process.Kill()
			<-waitCh
		}
		return nil
	}
}

func min(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
