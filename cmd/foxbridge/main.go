package main

import (
	"encoding/json"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/PopcornDev1/foxbridge/pkg/bridge"
	"github.com/PopcornDev1/foxbridge/pkg/cdp"
	"github.com/PopcornDev1/foxbridge/pkg/firefox"
)

func main() {
	port := flag.Int("port", 9222, "CDP WebSocket port")
	binary := flag.String("binary", "", "Firefox/Camoufox binary path")
	headless := flag.Bool("headless", false, "Run headless")
	profile := flag.String("profile", "", "Firefox profile directory")
	flag.Parse()

	// 1. Launch Firefox.
	proc := firefox.New()
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

	log.Printf("firefox started (PID %d)", proc.PID())

	// 2. Get the Juggler backend.
	backend := proc.Client()

	// 3. Create CDP session manager and server.
	sessions := cdp.NewSessionManager()

	var b *bridge.Bridge
	server := cdp.NewServer(*port, func(conn *cdp.Connection, msg *cdp.Message) {
		b.HandleMessage(conn, msg)
	})

	// 4. Create bridge and set up Juggler → CDP event subscriptions BEFORE enabling Browser.
	// This ensures attachedToTarget for the initial tab is captured.
	b = bridge.New(backend, sessions, server)
	b.SetupEventSubscriptions()

	// 5. Enable Browser domain with attachToDefaultContext.
	// This triggers Browser.attachedToTarget for the initial about:blank page.
	enableParams, _ := json.Marshal(map[string]interface{}{
		"attachToDefaultContext": true,
	})
	_, err = backend.Call("", "Browser.enable", enableParams)
	if err != nil {
		log.Fatalf("failed to enable Browser domain: %v", err)
	}

	// 6. Start server in background.
	go func() {
		if err := server.Start(); err != nil {
			log.Fatalf("CDP server error: %v", err)
		}
	}()

	log.Printf("foxbridge CDP proxy listening on 127.0.0.1:%d", *port)

	// 7. Wait for signal or Firefox exit.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	done := make(chan struct{})
	go func() {
		proc.Wait()
		close(done)
	}()

	select {
	case <-sig:
		log.Println("shutting down...")
	case <-done:
		log.Println("firefox exited")
	}
}
