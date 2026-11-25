#!/bin/sh
# Redis can accept TCP connections while it is still loading the AOF. A failed
# export should never be published or make us wait until the next day.
set -eu
umask 077
export LC_ALL=C

dir=${REDIS_BACKUP_DIR:-/backups}
host=${REDIS_BACKUP_HOST:-redis}
interval=86400
retry=900
max_age=93600 # 26 hours: one daily run, plus startup/retry grace
pending=''
trap '[ -z "$pending" ] || rm -f -- "$pending"' EXIT
trap 'exit 1' HUP INT TERM

backup() {
    stamp=$(date -u +%Y%m%d-%H%M%S) || return 1
    pending=$(mktemp "$dir/.dump-XXXXXXXX") || return 1
    if timeout 600 redis-cli -h "$host" --rdb "$pending" &&
        [ -s "$pending" ] && redis-check-rdb "$pending" &&
        mv -- "$pending" "$dir/dump-$stamp.rdb"; then
        pending=''
        echo "backup ok: dump-$stamp.rdb"
        # The filename is UTC and sortable; never touch manual dumps.
        set -- "$dir"/dump-[0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9]-[0-9][0-9][0-9][0-9][0-9][0-9].rdb
        while [ "$#" -gt 14 ]; do
            rm -f -- "$1" || echo "backup rotation failed: ${1##*/}" >&2
            shift
        done
        return 0
    fi
    echo "backup FAILED: $stamp" >&2
    rm -f -- "$pending" || return 1
    pending=''
    return 1
}

check_recent() {
    now=$(date +%s) || return 1
    for dump in "$dir"/dump-????????-??????.rdb; do
        [ -s "$dump" ] || continue
        modified=$(stat -c '%Y' -- "$dump") || continue
        age=$((now - modified))
        if [ "$age" -ge 0 ] && [ "$age" -le "$max_age" ]; then return 0; fi
    done
    echo "No recent Redis backup (maximum age: 26 h)" >&2
    return 1
}

case ${1:-} in
    --check) check_recent; exit $? ;;
    --once) backup; exit $? ;;
    '') ;;
    *) echo "Usage: $0 [--once|--check]" >&2; exit 2 ;;
esac

while :; do
    if backup; then sleep "$interval"; else sleep "$retry"; fi
done
