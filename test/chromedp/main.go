//go:build ignore

// Chromedp test for foxbridge — tests CDP from a Go client
//
// Prerequisites: foxbridge running on port 9222
// Usage: go run test/chromedp/main.go

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/chromedp/chromedp"
)

func main() {
	wsURL := "ws://127.0.0.1:9222/devtools/browser/foxbridge"
	passed, failed := 0, 0

	test := func(name string, fn func() error) {
		fmt.Printf("  %s... ", name)
		if err := fn(); err != nil {
			fmt.Printf("❌ %s\n", err)
			failed++
		} else {
			fmt.Println("✅")
			passed++
		}
	}

	fmt.Println("=== Foxbridge Chromedp Test Suite ===")
	fmt.Printf("Connecting to %s...\n\n", wsURL)

	// Connect
	allocCtx, cancel := chromedp.NewRemoteAllocator(context.Background(), wsURL)
	defer cancel()

	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	// Set a global timeout
	ctx, cancel = context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	fmt.Println("🔌 Connection")
	test("connect and create tab", func() error {
		return chromedp.Run(ctx)
	})

	fmt.Println("\n📄 Navigation")
	test("navigate to example.com", func() error {
		return chromedp.Run(ctx, chromedp.Navigate("https://example.com"))
	})

	var title string
	test("get page title", func() error {
		err := chromedp.Run(ctx, chromedp.Title(&title))
		if err != nil {
			return err
		}
		if title == "" {
			return fmt.Errorf("title is empty")
		}
		fmt.Printf("(%s) ", title)
		return nil
	})

	fmt.Println("\n🧮 Evaluation")
	var result string
	test("evaluate JS expression", func() error {
		return chromedp.Run(ctx, chromedp.Evaluate(`document.title`, &result))
	})

	test("evaluate with return value", func() error {
		var num int
		err := chromedp.Run(ctx, chromedp.Evaluate(`1 + 2 + 3`, &num))
		if err != nil {
			return err
		}
		if num != 6 {
			return fmt.Errorf("expected 6, got %d", num)
		}
		return nil
	})

	fmt.Println("\n🔍 DOM Queries")
	var text string
	test("query selector text", func() error {
		return chromedp.Run(ctx, chromedp.Text("h1", &text))
	})

	var html string
	test("outer HTML", func() error {
		return chromedp.Run(ctx, chromedp.OuterHTML("body", &html))
	})

	fmt.Println("\n📸 Screenshot")
	var buf []byte
	test("capture screenshot", func() error {
		err := chromedp.Run(ctx, chromedp.CaptureScreenshot(&buf))
		if err != nil {
			return err
		}
		if len(buf) < 100 {
			return fmt.Errorf("screenshot too small: %d bytes", len(buf))
		}
		fmt.Printf("(%d bytes) ", len(buf))
		return nil
	})

	fmt.Println("\n⌨️ Input")
	test("navigate to input page and type", func() error {
		return chromedp.Run(ctx,
			chromedp.Navigate("data:text/html,<input id='test' autofocus>"),
			chromedp.WaitVisible("#test"),
			chromedp.SendKeys("#test", "hello chromedp"),
		)
	})

	var inputVal string
	test("verify typed text", func() error {
		err := chromedp.Run(ctx, chromedp.Value("#test", &inputVal))
		if err != nil {
			return err
		}
		if inputVal != "hello chromedp" {
			return fmt.Errorf("expected 'hello chromedp', got '%s'", inputVal)
		}
		return nil
	})

	test("click element", func() error {
		return chromedp.Run(ctx,
			chromedp.Navigate("data:text/html,<button id='btn' onclick=\"document.title='clicked'\">Click</button>"),
			chromedp.WaitVisible("#btn"),
			chromedp.Click("#btn"),
			chromedp.Sleep(500*time.Millisecond),
		)
	})

	var clickTitle string
	test("verify click result", func() error {
		err := chromedp.Run(ctx, chromedp.Title(&clickTitle))
		if err != nil {
			return err
		}
		if clickTitle != "clicked" {
			return fmt.Errorf("expected 'clicked', got '%s'", clickTitle)
		}
		return nil
	})

	fmt.Printf("\n%s\n", "==================================================")
	fmt.Printf("Results: %d passed, %d failed\n", passed, failed)
	fmt.Printf("%s\n", "==================================================")

	if failed > 0 {
		os.Exit(1)
	}
}
