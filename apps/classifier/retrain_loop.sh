#!/usr/bin/env bash
# Continuous bootstrap -> safety-check -> train loop for the relevance classifier.
# Stops itself (does not train) if the CSAM term check finds any match —
# that requires human review, not an automatic decision.
set -uo pipefail

cd "$(dirname "$0")"
export MONGO_HOST="${MONGO_HOST:-localhost}"

INTERVAL_SECONDS="${RETRAIN_INTERVAL_SECONDS:-600}"
LOG_FILE="retrain_loop.log"

log() {
    printf '%s %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$1" | tee -a "$LOG_FILE"
}

log "=== retrain loop started (interval=${INTERVAL_SECONDS}s) ==="

while true; do
    log "bootstrap: fetching corpus and relabeling"
    if ! go run ./cmd/bootstrap >>"$LOG_FILE" 2>&1; then
        log "bootstrap FAILED — stopping loop, see $LOG_FILE"
        exit 1
    fi

    log "safety check: scanning corpus for known CSAM terms"
    check_output="$(go run ./cmd/checkfilter 2>&1)"
    echo "$check_output" >>"$LOG_FILE"
    match_count="$(echo "$check_output" | grep -o 'total matching docs: [0-9]*' | grep -o '[0-9]*$')"

    if [ -n "$match_count" ] && [ "$match_count" -gt 0 ]; then
        log "!!! SAFETY STOP: found ${match_count} documents matching CSAM terms. Loop halted. Do NOT resume until a human has reviewed and purged this content. !!!"
        exit 2
    fi

    log "safety check clear (0 matches) — proceeding to train"
    if ! go run ./cmd/train >>"$LOG_FILE" 2>&1; then
        log "train FAILED — stopping loop, see $LOG_FILE"
        exit 1
    fi
    tail -n 2 "$LOG_FILE"

    log "sleeping ${INTERVAL_SECONDS}s before next cycle"
    sleep "$INTERVAL_SECONDS"
done
