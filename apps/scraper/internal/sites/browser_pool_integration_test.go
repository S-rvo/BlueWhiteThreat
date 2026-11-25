package sites

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

func TestPooledBrowserSurvivesTwoJobsAndRebuildsAfterExit(t *testing.T) {
	if os.Getenv("BWT_CHECK_POOLED_CHROMIUM") != "1" {
		t.Skip("requires disposable, sandboxed Chromium")
	}
	t.Setenv("BWT_BROWSER_SECRET_PROBE", "not-for-chromium")
	parent, stop := context.WithTimeout(context.Background(), 45*time.Second)
	defer stop()
	browser := newPooledBrowserContext(parent, "127.0.0.1:9")
	defer browser.close()
	visit := func(title string) int {
		t.Helper()
		tab, closeTab, err := browser.newTab()
		if err != nil {
			t.Fatal(err)
		}
		defer closeTab()
		job, cancel := context.WithTimeout(tab, 10*time.Second)
		defer cancel()
		var got string
		err = chromedp.Run(job, chromedp.Navigate("data:text/html,<title>"+title+"</title>"), chromedp.Title(&got))
		if err != nil || got != title {
			t.Fatalf("visit %q, title %q: %v", title, got, err)
		}
		return chromedp.FromContext(tab).Browser.Process().Pid
	}
	first := visit("premiere")
	if os.Getenv("SCRAPER_CHROMIUM_EXECUTABLE") == "/usr/local/bin/bwt-chromium" {
		env, err := os.ReadFile(fmt.Sprintf("/proc/%d/environ", first))
		if err != nil || bytes.Contains(env, []byte("BWT_BROWSER_SECRET_PROBE=")) {
			t.Fatalf("browser environment not isolated: %v", err)
		}
	}
	second := visit("deuxieme")
	if first != second {
		t.Fatalf("two jobs used different browser processes: %d, %d", first, second)
	}
	browser.cancel()
	third := visit("troisieme")
	if third == second {
		t.Fatalf("browser was not rebuilt after exit: %d", third)
	}
}
