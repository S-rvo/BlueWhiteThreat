#!/usr/bin/env python3
"""Aggregate complete segment consensus without inventing parent labels."""
import argparse
import hashlib
import json
from collections import defaultdict
from pathlib import Path


def aggregate(documents, consensus):
    by_parent = defaultdict(list)
    text_hashes = {}
    for document in documents:
        parent = document.get("parent_id")
        if not parent:
            raise ValueError("segment missing parent_id")
        by_parent[parent].append(document["id"])
        text_hashes[document["id"]] = hashlib.sha256(
            document["text"].encode()
        ).hexdigest()

    votes = {item["id"]: item for item in consensus["documents"]}
    result = []
    for parent, segment_ids in sorted(by_parent.items()):
        items = []
        for segment_id in segment_ids:
            item = votes.get(segment_id)
            if item is None:
                raise ValueError(f"missing annotation for {segment_id}")
            if item.get("text_sha256") != text_hashes[segment_id]:
                raise ValueError(f"text hash mismatch for {segment_id}")
            items.append(item)

        statuses = {item["status"] for item in items}
        labels = {item["label"] for item in items if item["status"] == "consensus"}
        if statuses == {"consensus"} and len(labels) == 1:
            status, label = "consensus", labels.pop()
        elif statuses == {"consensus"}:
            status, label = "mixed_segments", None
        else:
            status, label = "abstention", None
        result.append({
            "id": parent,
            "label": label,
            "status": status,
            "segments": len(items),
            "segment_ids": sorted(segment_ids),
        })
    return {"documents": result, "policy": (
        "parent consensus only when every segment is consensus with the same label; "
        "mixed labels and incomplete certainty remain unresolved"
    )}


def load_jsonl(path):
    with path.open() as source:
        return [json.loads(line) for line in source if line.strip()]


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--segments", type=Path, required=True)
    parser.add_argument("--consensus", type=Path, required=True)
    parser.add_argument("--out", type=Path, required=True)
    args = parser.parse_args()
    payload = aggregate(load_jsonl(args.segments), json.loads(args.consensus.read_text()))
    with args.out.open("x") as destination:
        json.dump(payload, destination, ensure_ascii=False, indent=2)
        destination.write("\n")
    print(json.dumps({"parents": len(payload["documents"])}))


if __name__ == "__main__":
    main()
