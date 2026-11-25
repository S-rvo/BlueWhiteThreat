#!/bin/sh
# chromedp inherits the service environment. Scrub it before the browser
# itself starts: the renderer does not require the service's credentials.
set -eu
exec /usr/bin/env -i \
    "PATH=/usr/local/bin:/usr/bin:/bin" \
    "HOME=${HOME:-/tmp}" \
    "TMPDIR=${TMPDIR:-/tmp}" \
    "LANG=${LANG:-C.UTF-8}" \
    "XDG_CACHE_HOME=${XDG_CACHE_HOME:-/tmp/cache}" \
    "XDG_CONFIG_HOME=${XDG_CONFIG_HOME:-/tmp/config}" \
    /usr/bin/chromium "$@"
