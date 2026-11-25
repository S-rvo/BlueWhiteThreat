#!/usr/bin/env python3
"""Report annotator agreement without confusing majority coverage with unanimity."""
import argparse
import itertools
import json
from pathlib import Path


def read_votes(path):
    votes = {}
    for line in Path(path).read_text().splitlines():
        if not line.strip():
            continue
        row = json.loads(line)
        votes.setdefault(row["id"], {})[row["annotator_model"]] = row
    return votes


def audit(votes, models):
    per_doc = []
    pair_counts = {f"{a}__{b}": {"agree": 0, "n": 0} for a, b in itertools.combinations(models, 2)}
    for doc_id, ballot in sorted(votes.items()):
        decided = {m: ballot[m]["label"] for m in models
                   if m in ballot and ballot[m].get("status") == "decided"
                   and ballot[m].get("label") in (0, 1)}
        labels = list(decided.values())
        if len(labels) == len(models) and len(set(labels)) == 1:
            kind = "unanimous"
        elif len(labels) == len(models) and max(labels.count(0), labels.count(1)) >= 2:
            kind = "majority_only"
        elif len(labels) >= 2:
            kind = "no_majority_or_partial"
        else:
            kind = "insufficient_votes"
        per_doc.append({"id": doc_id, "kind": kind, "decided": len(labels)})
        for a, b in itertools.combinations(models, 2):
            if a in decided and b in decided:
                pair = pair_counts[f"{a}__{b}"]
                pair["n"] += 1
                pair["agree"] += decided[a] == decided[b]
    for pair in pair_counts.values():
        pair["agreement"] = pair["agree"] / pair["n"] if pair["n"] else None
    criteria_names = sorted({name for ballot in votes.values() for row in ballot.values()
                             for name in row.get("criteria", {})})
    criteria_report = {}
    for criterion in criteria_names:
        positives = {model: sum(
            1 for ballot in votes.values()
            if model in ballot and ballot[model].get("status") == "decided"
            and isinstance(ballot[model].get("criteria"), dict)
            and ballot[model]["criteria"].get(criterion) is True)
            for model in models}
        pairs = {}
        for a, b in itertools.combinations(models, 2):
            compared = [(ballot[a]["criteria"].get(criterion), ballot[b]["criteria"].get(criterion))
                        for ballot in votes.values()
                        if a in ballot and b in ballot
                        and ballot[a].get("status") == "decided" and ballot[b].get("status") == "decided"
                        and isinstance(ballot[a].get("criteria"), dict)
                        and isinstance(ballot[b].get("criteria"), dict)
                        and type(ballot[a]["criteria"].get(criterion)) is bool
                        and type(ballot[b]["criteria"].get(criterion)) is bool]
            agree = sum(x == y for x, y in compared)
            pairs[f"{a}__{b}"] = {"agree": agree, "n": len(compared),
                                  "agreement": agree / len(compared) if compared else None}
        criteria_report[criterion] = {"positive_counts": positives, "pairwise": pairs}
    counts = {name: sum(doc["kind"] == name for doc in per_doc)
              for name in ("unanimous", "majority_only", "no_majority_or_partial", "insufficient_votes")}
    return {"documents": len(per_doc), "counts": counts, "pairwise": pair_counts,
            "criteria": criteria_report, "documents_by_agreement": per_doc}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("votes", type=Path)
    parser.add_argument("--models", nargs="+", required=True)
    parser.add_argument("--out", type=Path)
    args = parser.parse_args()
    report = audit(read_votes(args.votes), args.models)
    payload = json.dumps(report, indent=2, sort_keys=True) + "\n"
    if args.out:
        args.out.write_text(payload)
    else:
        print(payload, end="")


if __name__ == "__main__":
    main()
