#!/usr/bin/env python3
"""Build a balanced consensus-only corpus and emit a leakage/source audit."""
import argparse
import csv
import hashlib
import json
import re
from collections import Counter
from pathlib import Path


def load_jsonl(path):
    return [json.loads(line) for line in path.open() if line.strip()]


def normalized(text):
    return re.sub(r"\s+", " ", text).strip().lower()


def fingerprint(text):
    """Mirror internal/overlap: normalized text plus 5-token shingles."""
    tokens = re.findall(r"[a-z0-9]+", normalized(text))
    return (" ".join(tokens), len(tokens), {tuple(tokens[i:i + 5]) for i in range(max(0, len(tokens) - 4))})


def similar(a, b):
    if a[0] == b[0]:
        return True
    if a[1] < 20 or b[1] < 20 or not a[2] or not b[2]:
        return False
    return len(a[2] & b[2]) / len(a[2] | b[2]) >= .80


def annotated(pool_path, consensus_path, wanted):
    docs = {x["id"]: x for x in load_jsonl(pool_path)}
    consensus = json.loads(consensus_path.read_text())["documents"]
    return [docs[x["id"]] | {"label": wanted, "annotation_status": "consensus"}
            for x in consensus if x["status"] == "consensus" and x.get("label") == wanted]


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--positive-pool", type=Path, required=True)
    ap.add_argument("--positive-consensus", type=Path, required=True)
    ap.add_argument("--negative-pool", type=Path, required=True)
    ap.add_argument("--negative-consensus", type=Path, required=True)
    ap.add_argument("--supplemental-pool", type=Path)
    ap.add_argument("--supplemental-consensus", type=Path)
    ap.add_argument("--consumed-test", action="append", type=Path, default=[])
    ap.add_argument("--exclude-id-file", type=Path)
    ap.add_argument("--out", type=Path, required=True)
    ap.add_argument("--per-class", type=int, default=200)
    ap.add_argument("--source-caps", type=Path, help="JSON object keyed by label then source")
    args = ap.parse_args()
    review_exclusions = set()
    if args.exclude_id_file:
        review_exclusions = set(json.loads(args.exclude_id_file.read_text()).get("excluded_ids", []))

    consumed, consumed_groups = [], set()
    for path in args.consumed_test:
        for row in load_jsonl(path):
            if row.get("text"):
                consumed.append(fingerprint(row["text"]))
            if row.get("group"):
                consumed_groups.add(row["group"])
    consumed_exact = {print_[0] for print_ in consumed}

    positives = annotated(args.positive_pool, args.positive_consensus, 1)
    if args.supplemental_pool and args.supplemental_consensus:
        positives += annotated(args.supplemental_pool, args.supplemental_consensus, 1)
    negatives = annotated(args.negative_pool, args.negative_consensus, 0)
    caps = {
        1: {"cisa-kev-ransomware": 95, "github-advisories": 88,
            "cert-fr-cti": 11, "cert-eu-cti": 25, "target_observation": 2},
        0: {"superuser": 70, "askubuntu": 70, "security": 50,
            "target_observation": 10},
    }
    if args.source_caps:
        raw_caps = json.loads(args.source_caps.read_text())
        caps = {int(label): {str(source): int(cap) for source, cap in values.items()}
                for label, values in raw_caps.items()}
    # The ten independently annotated target negatives are added to the new pool.
    positives_source = {x["id"]: x for x in load_jsonl(args.positive_pool)}
    positive_consensus = json.loads(args.positive_consensus.read_text())["documents"]
    negatives += [positives_source[x["id"]] | {"label": 0, "annotation_status": "consensus"}
                  for x in positive_consensus if x["status"] == "consensus" and x.get("label") == 0]

    chosen, excluded = [], Counter()
    seen_text, seen_ids, seen_fingerprints = set(), set(), []
    source_counts = Counter()
    for label, candidates in ((1, positives), (0, negatives)):
        for item in sorted(candidates, key=lambda x: (x.get("source", ""), x["id"])):
            if sum(1 for x in chosen if x["label"] == label) >= args.per_class:
                break
            source = item.get("source", "unknown")
            text = normalized(item["text"])
            print_ = fingerprint(text)
            if item["id"] in review_exclusions:
                excluded["manual_audit"] += 1; continue
            if source_counts[(label, source)] >= caps[label].get(source, 0):
                excluded["source_cap"] += 1; continue
            if item.get("group") in consumed_groups or print_[0] in consumed_exact:
                excluded["consumed_exact_or_group"] += 1; continue
            if any(similar(print_, other) for other in consumed):
                excluded["consumed_near_duplicate"] += 1; continue
            if text in seen_text or item["id"] in seen_ids:
                excluded["corpus_duplicate"] += 1; continue
            if any(similar(print_, other) for other in seen_fingerprints):
                excluded["corpus_near_duplicate"] += 1; continue
            chosen.append(item)
            seen_text.add(text); seen_ids.add(item["id"])
            seen_fingerprints.append(print_)
            source_counts[(label, source)] += 1
        if sum(1 for x in chosen if x["label"] == label) != args.per_class:
            raise SystemExit(f"insufficient label {label}: {sum(1 for x in chosen if x['label']==label)}")

    chosen.sort(key=lambda x: (x["label"], x.get("source", ""), x["id"]))
    args.out.write_text("".join(json.dumps(x, ensure_ascii=False) + "\n" for x in chosen))
    manifest = {
        "documents": len(chosen), "labels": Counter(x["label"] for x in chosen),
        "sources": {f"label_{label}:{source}": count for (label, source), count in sorted(source_counts.items())},
        "excluded": excluded, "consumed_tests": [str(x) for x in args.consumed_test],
        "exact_duplicates": len(chosen) - len(seen_text), "source_caps": caps,
        "annotation_authority": "agreement of two frozen local annotators",
    }
    Path(str(args.out) + ".manifest.json").write_text(json.dumps(manifest, indent=2, default=dict) + "\n")
    with Path(str(args.out) + ".csv").open("w", newline="") as handle:
        writer = csv.DictWriter(handle, fieldnames=["text", "label", "id", "source", "group"])
        writer.writeheader()
        for x in chosen:
            writer.writerow({key: x.get(key, "") for key in writer.fieldnames})
    print(json.dumps(manifest, default=dict))


if __name__ == "__main__":
    main()
