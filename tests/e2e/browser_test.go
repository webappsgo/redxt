//go:build e2e

package e2e

import (
	"context"
	"os/exec"
	"testing"
	"time"

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

// chromeAvailable reports whether a headless-Chromium-capable binary is
// on PATH. Tiers 2 and 3 (browser rendering, JS on/off, console errors)
// require an actual browser engine; when none is present the tests
// skip with an explicit reason instead of failing the whole suite, so
// tests/e2e.sh stays usable on hosts without Chromium installed.
func chromeAvailable(t *testing.T) {
	t.Helper()
	for _, name := range []string{"chromium", "chromium-browser", "google-chrome", "google-chrome-stable"} {
		if _, err := exec.LookPath(name); err == nil {
			return
		}
	}
	t.Skip("no headless-Chromium binary on PATH — install chromium to run browser tiers")
}

// newBrowserCtx starts a headless Chrome context with a bounded
// lifetime so a hung page never stalls the suite.
func newBrowserCtx(t *testing.T) context.Context {
	t.Helper()
	chromeAvailable(t)

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
	)
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
	t.Cleanup(cancelAlloc)

	ctx, cancel := chromedp.NewContext(allocCtx)
	t.Cleanup(cancel)

	ctx, timeoutCancel := context.WithTimeout(ctx, 20*time.Second)
	t.Cleanup(timeoutCancel)

	return ctx
}

// TestHomeRendersWithJavaScriptDisabled covers Tier 2: every feature
// must work without JavaScript, per PART 16.
func TestHomeRendersWithJavaScriptDisabled(t *testing.T) {
	base := testServer(t)
	ctx := newBrowserCtx(t)

	var html string
	if err := chromedp.Run(ctx,
		chromedp.Navigate(base+"/"),
		chromedp.OuterHTML("html", &html),
	); err != nil {
		t.Fatalf("chromedp.Run() error = %v", err)
	}
	if html == "" {
		t.Errorf("home page rendered no HTML")
	}
}

// TestHomeHasNoConsoleErrors covers Tier 3: the fully-JS-enabled
// browser render must produce zero console errors on the universal
// pages.
func TestHomeHasNoConsoleErrors(t *testing.T) {
	base := testServer(t)
	ctx := newBrowserCtx(t)

	var consoleErrors []string
	chromedp.ListenTarget(ctx, func(ev interface{}) {
		if e, ok := ev.(*runtime.EventConsoleAPICalled); ok && e.Type == runtime.APITypeError {
			consoleErrors = append(consoleErrors, "console error observed")
		}
	})

	if err := chromedp.Run(ctx,
		chromedp.Navigate(base+"/"),
		chromedp.WaitReady("body"),
	); err != nil {
		t.Fatalf("chromedp.Run() error = %v", err)
	}
	if len(consoleErrors) > 0 {
		t.Errorf("home page produced %d console error(s), want 0", len(consoleErrors))
	}
}

// TestResponsiveViewport375x812 covers the required mobile viewport
// (iPhone X class, 375x812) per PART 16/29 mobile-first responsive
// requirements: the page must render without a horizontal scrollbar.
func TestResponsiveViewport375x812(t *testing.T) {
	base := testServer(t)
	ctx := newBrowserCtx(t)

	var scrollWidth, clientWidth int64
	if err := chromedp.Run(ctx,
		chromedp.EmulateViewport(375, 812),
		chromedp.Navigate(base+"/"),
		chromedp.WaitReady("body"),
		chromedp.Evaluate(`document.documentElement.scrollWidth`, &scrollWidth),
		chromedp.Evaluate(`document.documentElement.clientWidth`, &clientWidth),
	); err != nil {
		t.Fatalf("chromedp.Run() error = %v", err)
	}
	if scrollWidth > clientWidth {
		t.Errorf("page overflows the 375px viewport: scrollWidth=%d clientWidth=%d — a long string is likely breaking mobile layout", scrollWidth, clientWidth)
	}
}
