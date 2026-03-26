package firefox

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/PopcornDev1/foxbridge/pkg/backend"
	"github.com/PopcornDev1/foxbridge/pkg/backend/juggler"
)

// Config holds the Firefox launch configuration.
type Config struct {
	// Path to the Firefox/Camoufox binary. If empty, auto-detected.
	BinaryPath string
	// Extra Firefox arguments.
	ExtraArgs []string
	// Headless mode.
	Headless bool
	// Profile directory.
	ProfileDir string
	// BiDiPort enables WebDriver BiDi on this port. 0 = disabled.
	BiDiPort int
}

// Process manages a Firefox/Camoufox browser process.
type Process struct {
	cmd              *exec.Cmd
	client           *juggler.Client
	transport        *juggler.PipeTransport
	bidiPort         int
	startedAt time.Time
	waited           bool
	mu               sync.Mutex
}

// New creates a new Process instance without starting it.
func New() *Process {
	return &Process{}
}

// Start launches Firefox and establishes the Juggler pipe connection.
func (p *Process) Start(cfg Config) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.cmd != nil {
		return fmt.Errorf("process already running")
	}

	bin := cfg.BinaryPath
	if bin == "" {
		var err error
		bin, err = findBinary()
		if err != nil {
			return err
		}
	}

	args := []string{
		"--no-remote",
	}

	// BiDi-only mode: don't use Juggler pipes (standard Firefox doesn't have Juggler)
	biDiOnly := cfg.BiDiPort > 0

	if !biDiOnly {
		args = append(args, "--juggler-pipe", "--purgecaches")
	}
	if cfg.Headless {
		args = append(args, "--headless")
	}
	if cfg.ProfileDir != "" {
		args = append(args, "--profile", cfg.ProfileDir)
	}
	if cfg.BiDiPort > 0 {
		args = append(args, fmt.Sprintf("--remote-debugging-port=%d", cfg.BiDiPort))
	}
	args = append(args, cfg.ExtraArgs...)

	cmd := exec.Command(bin, args...)
	// Suppress Firefox output
	devNull, _ := os.Open(os.DevNull)
	cmd.Stdout = devNull
	cmd.Stderr = devNull

	if !biDiOnly {
		// Create pipes for Juggler transport (FD 3 read, FD 4 write from Firefox's perspective).
		toFirefoxRead, toFirefoxWrite, err := os.Pipe()
		if err != nil {
			return fmt.Errorf("create pipe to firefox: %w", err)
		}
		fromFirefoxRead, fromFirefoxWrite, err := os.Pipe()
		if err != nil {
			toFirefoxRead.Close()
			toFirefoxWrite.Close()
			return fmt.Errorf("create pipe from firefox: %w", err)
		}

		// FD 0=stdin, 1=stdout, 2=stderr, 3=juggler-read (from us), 4=juggler-write (to us)
		cmd.ExtraFiles = []*os.File{toFirefoxRead, fromFirefoxWrite}

		if err := cmd.Start(); err != nil {
			toFirefoxRead.Close()
			toFirefoxWrite.Close()
			fromFirefoxRead.Close()
			fromFirefoxWrite.Close()
			return fmt.Errorf("start firefox: %w", err)
		}

		// Close the ends we don't use.
		toFirefoxRead.Close()
		fromFirefoxWrite.Close()

		transport := juggler.NewPipeTransport(fromFirefoxRead, toFirefoxWrite)
		client := juggler.NewClient(transport)

		p.client = client
		p.transport = transport
	} else {
		// BiDi-only: no pipes, just start the process
		if err := cmd.Start(); err != nil {
			return fmt.Errorf("start firefox: %w", err)
		}
	}

	p.cmd = cmd
	p.bidiPort = cfg.BiDiPort
	p.startedAt = time.Now()

	return nil
}

// Client returns the Juggler backend.
func (p *Process) Client() backend.Backend {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.client
}

// PID returns the Firefox process ID, or 0 if not running.
func (p *Process) PID() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cmd != nil && p.cmd.Process != nil {
		return p.cmd.Process.Pid
	}
	return 0
}

// BiDiURL returns the WebDriver BiDi WebSocket URL, or empty if BiDi is not enabled.
func (p *Process) BiDiURL() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.bidiPort > 0 {
		return fmt.Sprintf("ws://127.0.0.1:%d/session", p.bidiPort)
	}
	return ""
}

// Running returns true if the process is alive.
func (p *Process) Running() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.cmd != nil && p.cmd.ProcessState == nil
}

// Stop gracefully shuts down the Firefox process.
func (p *Process) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.client != nil {
		p.client.Call("", "Browser.close", nil)
		p.client.Close()
		p.client = nil
	}

	if p.cmd != nil && p.cmd.Process != nil && !p.waited {
		done := make(chan error, 1)
		go func() { done <- p.cmd.Wait() }()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			p.cmd.Process.Kill()
			<-done
		}
		p.waited = true
		p.cmd = nil
	}

	return nil
}

// Wait blocks until the Firefox process exits.
func (p *Process) Wait() error {
	p.mu.Lock()
	cmd := p.cmd
	if cmd == nil || p.waited {
		p.mu.Unlock()
		return nil
	}
	p.mu.Unlock()

	err := cmd.Wait()

	p.mu.Lock()
	p.waited = true
	p.mu.Unlock()

	return err
}

// findBinary locates a Firefox/Camoufox binary.
func findBinary() (string, error) {
	execPath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("get executable path: %w", err)
	}
	dir := filepath.Dir(execPath)

	candidates := []string{"camoufox", "camoufox-bin", "firefox", "firefox-bin"}
	if runtime.GOOS == "darwin" {
		candidates = append(candidates, "../MacOS/camoufox", "../MacOS/firefox")
	}
	if runtime.GOOS == "windows" {
		candidates = []string{"camoufox.exe", "firefox.exe"}
	}

	for _, name := range candidates {
		p := filepath.Join(dir, name)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}

	// Try PATH
	for _, name := range []string{"camoufox", "camoufox-bin", "firefox"} {
		if p, err := exec.LookPath(name); err == nil {
			return p, nil
		}
	}

	return "", fmt.Errorf("firefox/camoufox binary not found (looked in %s and PATH)", dir)
}
