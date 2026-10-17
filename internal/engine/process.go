package engine

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/Ehsanamn1/alpha/internal/config"
)

// Process supervises a user-configured local engine. Process launch is never
// implicit: the CLI requires an explicit --allow-process-launch flag.
type Process struct {
	cmd  *exec.Cmd
	done chan struct{}
	once sync.Once
}

func Start(spec config.EngineConfig) (*Process, error) {
	if !spec.StartProcess || spec.Command == "" {
		return nil, errors.New("engine process is not configured for launch")
	}
	cmd := exec.Command(spec.Command, spec.Args...)
	cmd.Dir = spec.WorkDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = nil
	cmd.Env = os.Environ()
	for k, v := range spec.Env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %q: %w", spec.Name, err)
	}
	p := &Process{cmd: cmd, done: make(chan struct{})}
	go func() {
		err := cmd.Wait()
		close(p.done)
		if err != nil {
			fmt.Fprintf(os.Stderr, "engine %q exited: %v\n", spec.Name, err)
		} else {
			fmt.Fprintf(os.Stderr, "engine %q exited\n", spec.Name)
		}
	}()
	return p, nil
}

func (p *Process) PID() int {
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return 0
	}
	return p.cmd.Process.Pid
}

func (p *Process) Stop() error {
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return nil
	}
	var stopErr error
	p.once.Do(func() {
		_ = p.cmd.Process.Signal(os.Interrupt)
		select {
		case <-p.done:
			return
		case <-time.After(1200 * time.Millisecond):
		}
		if err := p.cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			stopErr = err
		}
		select {
		case <-p.done:
		case <-time.After(time.Second):
			stopErr = errors.Join(stopErr, errors.New("timed out waiting for engine process to exit after kill"))
		}
	})
	return stopErr
}
