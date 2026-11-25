#!/usr/bin/env python3
"""Prepare the next training-only CTI supplement without reopening consumed tests."""
import csv
import json
from pathlib import Path

from annotate_autoeval import atomic_json, sha
from automatic_campaign import ROOT, fingerprints, lines, near, write_lines


def main():
    audit = ROOT / "data/source-audit-2026-10-19"
    pool_path = audit / "cti-pool.jsonl"
    consensus_path = audit / "cti-annotations/consensus.json"
    supplement_path = audit / "training-supplement.jsonl"
    combined_path = audit / "combined-weak-training.jsonl"
    plan_path = audit / "training-supplement-plan.json"
    for path in (supplement_path, combined_path, plan_path):
        if path.exists():
            raise ValueError(f"output_already_exists: {path}")

    pool = lines(pool_path)
    by_id = {d["id"]: d for d in pool}
    if len(by_id) != len(pool):
        raise ValueError("duplicate_pool_id")
    consensus = json.loads(consensus_path.read_text())
    positive_ids = {
        d["id"] for d in consensus["documents"]
        if d["status"] == "consensus" and d["label"] == 1
    }

    campaign = ROOT / "data/automatic-v2-2026-10-19"
    prior_paths = [
        campaign / "development.jsonl",
        campaign / "campaign/validation.jsonl",
        campaign / "campaign/test.jsonl",
        campaign / "retraining/test.jsonl",
        campaign / "retraining/weak-training.jsonl",
    ]
    prior_docs = []
    prior_groups = set()
    for path in prior_paths:
        for doc in lines(path):
            prior_docs.append(doc)
            if doc.get("group"):
                prior_groups.add(doc["group"])
    with (ROOT / "training_data.csv").open() as handle:
        prior_docs.extend(csv.DictReader(handle))

    binary = Path("/tmp/bwt-evaldocuments")
    if not binary.exists():
        raise ValueError("missing /tmp/bwt-evaldocuments; build cmd/evaldocuments first")
    prior_prints = list(fingerprints(binary, prior_docs).values())
    pool_prints = fingerprints(binary, pool)
    selected = []
    selected_prints = []
    excluded_group = excluded_near = 0
    for doc_id in sorted(positive_ids):
        doc = by_id[doc_id]
        if doc["group"] in prior_groups:
            excluded_group += 1
            continue
        fingerprint = pool_prints[doc_id]
        if any(near(fingerprint, old) for old in prior_prints + selected_prints):
            excluded_near += 1
            continue
        selected.append({
            **doc,
            "label": 1,
            "label_authority": "agreement of two local AI models; training only; not independent truth",
        })
        selected_prints.append(fingerprint)

    previous = lines(campaign / "retraining/weak-training.jsonl")
    write_lines(supplement_path, selected)
    write_lines(combined_path, previous + selected)
    atomic_json(plan_path, {
        "purpose": "next candidate training only",
        "old_consumed_tests_used_as_training": False,
        "requires_new_independent_test": True,
        "source_documents": len(pool),
        "AI_consensus_positives": len(positive_ids),
        "selected_novel_positives": len(selected),
        "excluded_existing_group": excluded_group,
        "excluded_near_duplicate": excluded_near,
        "previous_weak_positives": len(previous),
        "combined_weak_positives": len(previous) + len(selected),
        "input_sha256": {
            pool_path.name: sha(pool_path.read_bytes()),
            "consensus.json": sha(consensus_path.read_bytes()),
        },
    }, exclusive=True)
    print(json.dumps(json.loads(plan_path.read_text())))


if __name__ == "__main__":
    main()
