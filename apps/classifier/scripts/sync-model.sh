#!/usr/bin/env sh
# apps/classifier/scripts/sync-model.sh
#
# Copies model.json (+ its evaluation report) from the repo working tree
# into the shared volume read by the classifier-prd container, whenever
# the source file's content changes. Deploying a new model version is
# just: train, review, commit+push model.json, then `git pull` on the
# deploy host. This script notices the change and the classifier
# container's own reload loop (CLASSIFIER_INTERVAL_MIN) then starts
# scoring with the new model without any restart.
set -eu

SRC_DIR="${SRC_DIR:-/src}"
DEST_DIR="${DEST_DIR:-/data}"
INTERVAL_SECONDS="${MODEL_SYNC_INTERVAL_SECONDS:-60}"
STATE_FILE="$DEST_DIR/.last_synced_sha256"

log() {
    printf '%s %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$1"
}

log "=== model sync started (interval=${INTERVAL_SECONDS}s, src=${SRC_DIR}, dest=${DEST_DIR}) ==="

while true; do
    if [ -f "$SRC_DIR/model.json" ]; then
        current_hash="$(sha256sum "$SRC_DIR/model.json" | cut -d' ' -f1)"
        last_hash=""
        [ -f "$STATE_FILE" ] && last_hash="$(cat "$STATE_FILE")"

        if [ "$current_hash" != "$last_hash" ]; then
            log "model.json changed (sha256 ${current_hash}) -- syncing"
            tmp_model="$DEST_DIR/.model.json.tmp"
            cp "$SRC_DIR/model.json" "$tmp_model"
            mv "$tmp_model" "$DEST_DIR/model.json"
            if [ -f "$SRC_DIR/model.json.evaluation.json" ]; then
                cp "$SRC_DIR/model.json.evaluation.json" "$DEST_DIR/model.json.evaluation.json"
            fi
            echo "$current_hash" > "$STATE_FILE"
            log "sync complete"
        fi
    else
        log "WARNING: $SRC_DIR/model.json not found, nothing to sync"
    fi

    sleep "$INTERVAL_SECONDS"
done
