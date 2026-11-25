#!/usr/bin/env python3
"""Audit frozen OOF errors and group-bootstrap uncertainty."""
import argparse
import collections
import json
import random
import re
import math
from pathlib import Path

CATEGORIES = {
    "marketplace_or_directory": re.compile(r"\b(market|directory|links?|vendors?|shop|forum)\b", re.I),
    "privacy_or_policy": re.compile(r"\b(privacy|policy|terms|cookies?|gdpr)\b", re.I),
    "finance_or_crypto": re.compile(r"\b(bitcoin|crypto|wallet|payment|finance|bank)\b", re.I),
    "technical_or_hosting": re.compile(r"\b(server|hosting|domain|login|account|support|install)\b", re.I),
    "security_vocabulary": re.compile(r"\b(ransomware|malware|breach|leak|database|credentials?|exploit)\b", re.I),
}


def metrics(rows, thresholds, fallback):
    tp = fp = fn = tn = 0
    for row in rows:
        predicted = row["score"] >= thresholds.get(row["source"], fallback)
        if row["label"] == 1:
            tp += predicted
            fn += not predicted
        else:
            fp += predicted
            tn += not predicted
    precision = tp / (tp + fp) if tp + fp else 0
    recall = tp / (tp + fn) if tp + fn else 0
    return {"precision": precision, "recall": recall,
            "f1": 2 * precision * recall / (precision + recall) if precision + recall else 0,
            "tp": tp, "fp": fp, "fn": fn, "tn": tn}


def interval(values):
    values = sorted(values)
    return [values[int(.025 * (len(values) - 1))], values[int(.975 * (len(values) - 1))]]


def bootstrap(rows, thresholds, fallback, repeats=1000):
    grouped = collections.defaultdict(list)
    for row in rows:
        grouped[(row["source"], row["group"])].append(row)
    keys = sorted(grouped)
    rng = random.Random(20261021)
    distributions = collections.defaultdict(list)
    for _ in range(repeats):
        sample = []
        for _ in keys:
            sample.extend(grouped[rng.choice(keys)])
        result = metrics(sample, thresholds, fallback)
        for name in ("precision", "recall", "f1"):
            distributions[name].append(result[name])
    return {name + "_95ci": interval(values) for name, values in distributions.items()}


def select_recall_threshold(rows, target=.9):
    candidates = sorted({row["score"] for row in rows} | {1.0})
    feasible = [(threshold, metrics(rows, {}, threshold)) for threshold in candidates]
    feasible = [(threshold, result) for threshold, result in feasible if result["recall"] >= target]
    return max(feasible, key=lambda item: item[0]) if feasible else (0.0, metrics(rows, {}, 0.0))


def threshold_stability(rows, repeats=1000):
    grouped = collections.defaultdict(list)
    for row in rows:
        grouped[row["group"]].append(row)
    keys = sorted(grouped)
    rng = random.Random(20261022)
    thresholds, precisions, recalls = [], [], []
    passes = 0
    attempts = 0
    while len(thresholds) < repeats and attempts < repeats * 10:
        attempts += 1
        sample = []
        sampled_keys = [rng.choice(keys) for _ in keys]
        for key in sampled_keys:
            sample.extend(grouped[key])
        # Select on the in-bag groups and measure only on groups never sampled.
        # This is an honest group out-of-bag estimate rather than resubstitution
        # on the same bootstrap draw.
        held = [row for key in keys if key not in set(sampled_keys) for row in grouped[key]]
        if not held:
            continue
        threshold, _ = select_recall_threshold(sample)
        result = metrics(held, {}, threshold)
        thresholds.append(threshold)
        precisions.append(result["precision"])
        recalls.append(result["recall"])
        passes += result["precision"] >= .5 and result["recall"] >= .9
    return {"method": "group_bootstrap_out_of_bag", "successful_repeats": len(thresholds),
            "threshold_95ci": interval(thresholds),
            "precision_95ci": interval(precisions), "recall_95ci": interval(recalls),
            "target_pass_rate": passes / repeats}


def fit_platt(rows, epochs=2000, learning_rate=.1):
    a, b = 1.0, 0.0
    values = [(math.log(max(1e-12, min(1 - 1e-12, row["score"])) /
                        (1 - max(1e-12, min(1 - 1e-12, row["score"])))), row["label"])
              for row in rows]
    for _ in range(epochs):
        grad_a = grad_b = 0.0
        for z, label in values:
            prediction = 1 / (1 + math.exp(-(a * z + b)))
            error = prediction - label
            grad_a += error * z
            grad_b += error
        a -= learning_rate * grad_a / len(values)
        b -= learning_rate * grad_b / len(values)
    return a, b


def calibration_metrics(probabilities, labels):
    n = len(labels)
    brier = sum((p - y) ** 2 for p, y in zip(probabilities, labels)) / n
    log_loss = -sum(y * math.log(max(p, 1e-12)) + (1-y) * math.log(max(1-p, 1e-12))
                    for p, y in zip(probabilities, labels)) / n
    ece = 0.0
    for bin_index in range(10):
        low, high = bin_index / 10, (bin_index + 1) / 10
        members = [(p, y) for p, y in zip(probabilities, labels)
                   if low <= p < high or (bin_index == 9 and p == 1)]
        if members:
            ece += len(members) / n * abs(sum(p for p, _ in members) / len(members) -
                                          sum(y for _, y in members) / len(members))
    return {"brier": brier, "log_loss": log_loss, "ece_10_bins": ece}


def nested_calibration(rows):
    if not rows or any("fold" not in row for row in rows):
        return {"available": False, "reason": "fold_metadata_missing"}
    probabilities, labels = [], []
    for fold in sorted({row["fold"] for row in rows}):
        training = [row for row in rows if row["fold"] != fold]
        held = [row for row in rows if row["fold"] == fold]
        a, b = fit_platt(training)
        for row in held:
            p = max(1e-12, min(1 - 1e-12, row["score"]))
            probabilities.append(1 / (1 + math.exp(-(a * math.log(p / (1-p)) + b))))
            labels.append(row["label"])
    return {"available": True, "folds": len({row["fold"] for row in rows}),
            **calibration_metrics(probabilities, labels)}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--selection", type=Path, required=True)
    parser.add_argument("--documents", type=Path, required=True)
    parser.add_argument("--out", type=Path, required=True)
    args = parser.parse_args()
    report = json.loads(args.selection.read_text())
    rows = report.get("selected_oof_predictions")
    if not rows:
        raise ValueError("selection_report_has_no_oof_predictions")
    documents = {d["id"]: d for line in args.documents.read_text().splitlines() if line.strip() for d in [json.loads(line)]}
    for row in rows:
        if not row.get("sampling_stream"):
            row["sampling_stream"] = documents.get(row["id"], {}).get("sampling_stream", "representative_development")
    thresholds = report.get("experimental_source_thresholds", {})
    fallback = report["selected"]["threshold"]
    false_positives = []
    false_negatives = []
    category_counts = collections.Counter()
    for row in rows:
        if row["label"] != 0 or row["score"] < thresholds.get(row["source"], fallback):
            continue
        text = documents.get(row["id"], {}).get("text", "")
        categories = [name for name, pattern in CATEGORIES.items() if pattern.search(text)] or ["uncategorized"]
        category_counts.update(categories)
        false_positives.append({**row, "categories": categories, "text_excerpt": re.sub(r"\s+", " ", text)[:240]})
    for row in rows:
        if row["label"] != 1 or row["score"] >= thresholds.get(row["source"], fallback):
            continue
        text = documents.get(row["id"], {}).get("text", "")
        false_negatives.append({**row, "text_excerpt": re.sub(r"\s+", " ", text)[:240]})
    darkweb = [row for row in rows if row["source"] == "darkweb_page"]
    representative = [row for row in rows if row["sampling_stream"] != "targeted_development_enrichment"]
    representative_darkweb = [row for row in representative if row["source"] == "darkweb_page"]
    targeted = [row for row in rows if row["sampling_stream"] == "targeted_development_enrichment"]
    result = {"documents": len(rows), "metrics": metrics(rows, thresholds, fallback),
              "group_bootstrap_1000": bootstrap(rows, thresholds, fallback),
              "darkweb_metrics": metrics(darkweb, thresholds, fallback),
              "darkweb_group_bootstrap_1000": bootstrap(darkweb, thresholds, fallback),
              "representative_stream_metrics": metrics(representative, thresholds, fallback),
              "representative_group_bootstrap_1000": bootstrap(representative, thresholds, fallback),
              "representative_darkweb_metrics": metrics(representative_darkweb, thresholds, fallback),
              "representative_darkweb_group_bootstrap_1000": bootstrap(representative_darkweb, thresholds, fallback),
              "representative_darkweb_threshold_stability_1000": threshold_stability(representative_darkweb),
              "representative_nested_calibration": nested_calibration(representative),
              "targeted_enrichment_metrics": metrics(targeted, thresholds, fallback),
              "false_positive_categories": dict(category_counts), "false_positives": false_positives,
              "false_negatives": false_negatives,
              "selection_report": str(args.selection), "test_read": False}
    args.out.write_text(json.dumps(result, indent=2, ensure_ascii=False) + "\n")
    print(json.dumps({k: result[k] for k in ("documents", "metrics", "group_bootstrap_1000", "darkweb_metrics", "darkweb_group_bootstrap_1000", "representative_stream_metrics", "representative_group_bootstrap_1000", "representative_darkweb_metrics", "representative_darkweb_group_bootstrap_1000", "representative_nested_calibration", "targeted_enrichment_metrics", "false_positive_categories")}))


if __name__ == "__main__":
    main()
