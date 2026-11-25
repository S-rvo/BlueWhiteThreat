#!/usr/bin/env python3
"""Freeze an older development stream and a newest chronological reserve."""
import argparse
import json
from pathlib import Path


def split(documents, reserve_size):
    if reserve_size < 1 or reserve_size >= len(documents):
        raise ValueError("reserve_size_must_leave_both_streams_nonempty")
    ordered = sorted(documents, key=lambda d: (d.get("collected_at", ""), d["id"]))
    boundary = len(ordered) - reserve_size
    return ordered[:boundary], ordered[boundary:]


def write_new(path, documents, stream):
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("x") as output:
        for document in documents:
            output.write(json.dumps({**document, "sampling_stream": stream}, ensure_ascii=False) + "\n")
    path.chmod(0o600)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--input", type=Path, required=True)
    parser.add_argument("--development-out", type=Path, required=True)
    parser.add_argument("--reserve-out", type=Path, required=True)
    parser.add_argument("--reserve-size", type=int, required=True)
    args = parser.parse_args()
    documents = [json.loads(line) for line in args.input.read_text().splitlines() if line.strip()]
    if not documents or len({d["id"] for d in documents}) != len(documents):
        raise ValueError("invalid_or_duplicate_documents")
    development, reserve = split(documents, args.reserve_size)
    write_new(args.development_out, development, "prospective_development")
    write_new(args.reserve_out, reserve, "chronological_reserve")
    print(json.dumps({"input": len(documents), "development": len(development), "reserve": len(reserve),
                      "reserve_min_collected_at": reserve[0].get("collected_at", ""),
                      "development_max_collected_at": development[-1].get("collected_at", "")}))


if __name__ == "__main__":
    main()
