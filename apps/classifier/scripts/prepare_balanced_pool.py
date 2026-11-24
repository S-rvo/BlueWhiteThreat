#!/usr/bin/env python3
"""Prepare an unlabelled, source-capped pool without reusing evaluation data."""
import argparse
import csv
import hashlib
import json
import re
import sys
from collections import Counter
from pathlib import Path


def norm(text):
    return re.sub(r"\s+", " ", text).strip().lower()


def digest(text):
    return hashlib.sha256(norm(text).encode()).hexdigest()


def rows(path):
    with path.open(errors="replace") as handle:
        for line in handle:
            try:
                item = json.loads(line)
            except (ValueError, TypeError):
                continue
            if isinstance(item, dict) and item.get("text"):
                yield item


def main():
    csv.field_size_limit(sys.maxsize)
    ap = argparse.ArgumentParser()
    ap.add_argument("--input", action="append", type=Path, required=True)
    ap.add_argument("--exclude-root", type=Path, required=True)
    ap.add_argument("--training-csv", type=Path, required=True)
    ap.add_argument("--out", type=Path, required=True)
    ap.add_argument("--per-source", type=int, default=80)
    args = ap.parse_args()

    excluded_ids, excluded_groups, excluded_hashes = set(), set(), set()
    for path in args.exclude_root.rglob("*.jsonl"):
        if path.resolve() in {p.resolve() for p in args.input} or path.resolve() == args.out.resolve():
            continue
        for item in rows(path):
            excluded_ids.add(str(item.get("id", "")))
            excluded_groups.add(str(item.get("group", "")))
            excluded_hashes.add(digest(item["text"]))
    with args.training_csv.open(errors="replace", newline="") as handle:
        for item in csv.DictReader(handle):
            text = item.get("text") or item.get("content") or ""
            if text:
                excluded_hashes.add(digest(text))

    selected, counts, seen_hashes = [], Counter(), set()
    for path in args.input:
        for item in rows(path):
            source = str(item.get("source", "unknown"))
            h = digest(item["text"])
            if counts[source] >= args.per_source or len(item["text"]) > 12000:
                continue
            if str(item.get("id", "")) in excluded_ids or str(item.get("group", "")) in excluded_groups:
                continue
            if h in excluded_hashes or h in seen_hashes:
                continue
            item["text_sha256"] = hashlib.sha256(item["text"].encode()).hexdigest()
            item["pool_role"] = "unlabelled_candidate"
            selected.append(item)
            seen_hashes.add(h)
            counts[source] += 1

    args.out.parent.mkdir(parents=True, exist_ok=True)
    with args.out.open("x") as handle:
        for item in selected:
            handle.write(json.dumps(item, ensure_ascii=False) + "\n")
    print(json.dumps({"selected": len(selected), "by_source": counts}, sort_keys=True))


if __name__ == "__main__":
    main()
