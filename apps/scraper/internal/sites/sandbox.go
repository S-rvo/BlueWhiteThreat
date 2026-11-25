package sites

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/chromedp/chromedp"
)

// CheckBrowserSandbox probes Chromium before processing any URL. Use a
// non-serving loopback proxy: even on unexpected requests the probe is local.
func CheckBrowserSandbox(ctx context.Context) error {
	if os.Getuid() == 0 {
		return fmt.Errorf("scraper must run as a non-root user")
	}
	opts := append(BuildChromedpOptions("127.0.0.1:9"), chromedp.Flag("lang", "en-US"))
	allocator, cancelAlloc := chromedp.NewExecAllocator(ctx, opts...)
	defer cancelAlloc()
	browser, cancelBrowser := chromedp.NewContext(allocator)
	defer cancelBrowser()
	probe, cancelProbe := context.WithTimeout(browser, 20*time.Second)
	defer cancelProbe()
	var ready bool
	if err := chromedp.Run(probe, chromedp.Navigate("chrome://sandbox"), chromedp.Poll(
		`(() => {
			const rows = Array.from(document.querySelectorAll("tr"), row =>
				Array.from(row.cells, cell => cell.textContent.trim().toLowerCase()));
			const has = (label, value) => rows.some(row => row[0] === label && row[1] === value);
			return (has('layer 1 sandbox', 'namespace') || has('namespace sandbox', 'yes')) &&
				has('pid namespaces', 'yes') && has('network namespaces', 'yes') &&
				(has('seccomp-bpf sandbox', 'yes') || has('seccomp-bpf enabled (chrome)', 'yes'));
		})()`, &ready, chromedp.WithPollingTimeout(10*time.Second))); err != nil {
		return fmt.Errorf("Chromium namespace/seccomp sandbox verification failed: %w", err)
	}
	if !ready {
		return fmt.Errorf("Chromium namespace/seccomp sandbox is inactive")
	}
	return nil
}
