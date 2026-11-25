#!/usr/bin/env bash
set -euo pipefail
root="$(cd "$(dirname "$0")/../../.." && pwd)"
name="bwt-render-policy-test-$$"
cleanup() {
 docker rm -f "$name-browser" "$name-redis" >/dev/null 2>&1 || true
 docker network rm "$name" >/dev/null 2>&1 || true
}
trap cleanup EXIT
docker network create "$name" >/dev/null
docker run -d --name "$name-redis" --network "$name" --network-alias fixture-redis redis:7-alpine >/dev/null
docker run --rm --name "$name-browser" --network "$name" -v "$root:/workspace:ro" -w /workspace/apps/scraper \
 -e SCRAPER_TEST_REDIS=fixture-redis:6379 -e CRAWLER_TEST_REDIS=fixture-redis:6379 -e CRAWLER_TEST_CHROME=/usr/bin/chromium \
 --entrypoint sh "${SCRAPER_TEST_IMAGE:-bluewhitethreat-scraper}" \
 -c 'go test -race -count=1 ./tests && go test -count=1 -v ./internal/sites -run "^(TestPublicRenderPolicyFixture|TestOnionProxyRenderFixture)$"'
