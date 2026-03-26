package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	backendpkg "github.com/PopcornDev1/foxbridge/pkg/backend"
	"github.com/PopcornDev1/foxbridge/pkg/backend/bidi"
	"github.com/PopcornDev1/foxbridge/pkg/bridge"
	"github.com/PopcornDev1/foxbridge/pkg/cdp"
	"github.com/PopcornDev1/foxbridge/pkg/firefox"
)

func main() {
	port := flag.Int("port", 9222, "CDP WebSocket port")
	binary := flag.String("binary", "", "Firefox/Camoufox binary path")
	headless := flag.Bool("headless", false, "Run headless")
	profile := flag.String("profile", "", "Firefox profile directory")
	backendMode := flag.String("backend", "juggler", "Backend protocol: juggler or bidi")
	bidiURL := flag.String("bidi-url", "", "BiDi WebSocket URL (for --backend bidi without launching Firefox)")
	bidiPort := flag.Int("bidi-port", 9223, "Port for BiDi WebSocket when auto-launching Firefox")
	flag.Parse()

	var be backendpkg.Backend
	var proc *firefox.Process

	switch *backendMode {
	case "juggler":
		// Launch Firefox with Juggler pipe transport
		proc = firefox.New()
		err := proc.Start(firefox.Config{
			BinaryPath: *binary,
			Headless:   *headless,
			ProfileDir: *profile,
			ExtraArgs:  flag.Args(),
		})
		if err != nil {
			log.Fatalf("failed to start firefox: %v", err)
		}
		defer proc.Stop()
		log.Printf("firefox started with Juggler backend (PID %d)", proc.PID())
		be = proc.Client()

	case "bidi":
		if *bidiURL != "" {
			// Connect to an existing BiDi WebSocket
			transport, err := bidi.Dial(*bidiURL)
			if err != nil {
				log.Fatalf("failed to connect to BiDi endpoint %s: %v", *bidiURL, err)
			}
			client := bidi.NewClient(transport)
			defer client.Close()
			be = client
			log.Printf("connected to BiDi endpoint: %s", *bidiURL)
		} else {
			// Launch Firefox with BiDi WebSocket enabled and connect to it.
			proc = firefox.New()
			err := proc.Start(firefox.Config{
				BinaryPath: *binary,
				Headless:   *headless,
				ProfileDir: *profile,
				BiDiPort:   *bidiPort,
				ExtraArgs:  flag.Args(),
			})
			if err != nil {
				log.Fatalf("failed to start firefox: %v", err)
			}
			defer proc.Stop()

			// Wait for the BiDi WebSocket port to become available.
			addr := fmt.Sprintf("127.0.0.1:%d", *bidiPort)
			log.Printf("firefox started (PID %d), waiting for BiDi on %s...", proc.PID(), addr)
			if err := waitForPort(addr, 15*time.Second); err != nil {
				log.Fatalf("BiDi port never became available: %v", err)
			}

			// Get BiDi URL (auto-discovered from Firefox stderr, or fallback)
			wsURL := proc.BiDiURL()
			transport, err := bidi.Dial(wsURL)
			if err != nil {
				log.Fatalf("failed to connect to BiDi endpoint %s: %v", wsURL, err)
			}
			client := bidi.NewClient(transport)
			defer client.Close()
			be = client
			log.Printf("connected to auto-discovered BiDi endpoint: %s", wsURL)
		}

	default:
		fmt.Fprintf(os.Stderr, "unknown backend: %s (use 'juggler' or 'bidi')\n", *backendMode)
		os.Exit(1)
	}

	// Create CDP session manager and server.
	sessions := cdp.NewSessionManager()

	var b *bridge.Bridge
	server := cdp.NewServer(*port, func(conn *cdp.Connection, msg *cdp.Message) {
		b.HandleMessage(conn, msg)
	}, sessions)

	// Create bridge and set up event subscriptions BEFORE enabling Browser.
	b = bridge.New(be, sessions, server, *backendMode == "bidi")
	b.SetupEventSubscriptions()

	// Enable Browser domain with attachToDefaultContext.
	enableParams, _ := json.Marshal(map[string]interface{}{
		"attachToDefaultContext": true,
	})
	_, err := be.Call("", "Browser.enable", enableParams)
	if err != nil {
		log.Fatalf("failed to enable Browser domain: %v", err)
	}

	// Start server in background.
	go func() {
		if err := server.Start(); err != nil {
			log.Fatalf("CDP server error: %v", err)
		}
	}()

	log.Printf("foxbridge CDP proxy listening on 127.0.0.1:%d (backend: %s)", *port, *backendMode)

	// Wait for signal or Firefox exit.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	done := make(chan struct{})
	if proc != nil {
		go func() {
			proc.Wait()
			close(done)
		}()
	}

	select {
	case <-sig:
		log.Println("shutting down...")
	case <-done:
		log.Println("firefox exited")
	}
}

// waitForPort polls a TCP address until it accepts connections or the timeout expires.
func waitForPort(addr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
		if err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for %s after %v", addr, timeout)
}
