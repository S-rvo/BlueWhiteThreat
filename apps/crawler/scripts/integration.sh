#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
suffix="bwt-crawler-test-$$"
cleanup() { docker rm -f "$suffix-redis" "$suffix-neo4j" >/dev/null 2>&1 || true; }
trap cleanup EXIT
docker run -d --name "$suffix-redis" -p 127.0.0.1::6379 redis:7-alpine >/dev/null
docker run -d --name "$suffix-neo4j" -p 127.0.0.1::7687 -e NEO4J_AUTH=neo4j/fixture-password neo4j:5.19 >/dev/null
export CRAWLER_TEST_REDIS="$(docker port "$suffix-redis" 6379)"
export CRAWLER_TEST_NEO4J="bolt://$(docker port "$suffix-neo4j" 7687)"
ready=0
for _ in {1..90}; do
 if docker exec "$suffix-neo4j" cypher-shell -u neo4j -p fixture-password 'RETURN 1' >/dev/null 2>&1; then ready=1; break; fi
 sleep 1
done
[[ "$ready" == 1 ]] || { echo 'Neo4j readiness timed out'; exit 1; }
go test -race -count=1 -v ./internal/db -run '^TestDisposableIntegration$'
