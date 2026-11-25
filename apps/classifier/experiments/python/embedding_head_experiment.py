#!/usr/bin/env python3
"""Cross-fit a small logistic head over frozen multilingual embeddings."""

import argparse
import csv
import json
import sys
from pathlib import Path

import numpy as np
from sentence_transformers import SentenceTransformer
from sklearn.linear_model import LogisticRegression
from sklearn.metrics import average_precision_score, precision_recall_fscore_support

MODEL = "sentence-transformers/paraphrase-multilingual-MiniLM-L12-v2"
REVISION = "b8ef00830037f9868450f778081ea683e900fe39"
csv.field_size_limit(sys.maxsize)


def read_historical(path):
    rows = list(csv.DictReader(path.open(encoding="utf-8")))
    return [row["text"] for row in rows], np.array([int(row["label"]) for row in rows])


def read_jsonl(path):
    rows = [json.loads(line) for line in path.open(encoding="utf-8") if line.strip()]
    return rows, [row["text"] for row in rows], np.array([int(row["label"]) for row in rows])


def folds(labels, count=5):
    positions = [0, 0]
    result = []
    for label in labels:
        result.append(positions[label] % count)
        positions[label] += 1
    return np.array(result)


def metrics(labels, scores, threshold):
    predicted = scores >= threshold
    precision, recall, f1, _ = precision_recall_fscore_support(
        labels, predicted, average="binary", zero_division=0
    )
    return {"precision": precision, "recall": recall, "f1": f1}


def select_threshold(labels, scores):
    choices = []
    for threshold in np.unique(np.append(scores, 1.0)):
        row = {"threshold": float(threshold), **metrics(labels, scores, threshold)}
        row["feasible"] = row["precision"] >= 0.5 and row["recall"] >= 0.9
        choices.append(row)
    return max(choices, key=lambda row: (row["feasible"], row["precision"] if row["feasible"] else row["f1"], row["recall"], row["threshold"]))


def run(args):
    base_texts, base_y = read_historical(args.historical)
    dev_rows, dev_texts, dev_y = read_jsonl(args.development)
    encoder = SentenceTransformer(MODEL, revision=REVISION, device=args.device)
    all_vectors = encoder.encode(base_texts + dev_texts, batch_size=args.batch_size, normalize_embeddings=True, show_progress_bar=True)
    base_x = all_vectors[: len(base_texts)]
    dev_x = all_vectors[len(base_texts) :]
    assignment = folds(dev_y)
    trials = []
    for c in (0.1, 1.0, 10.0):
        for positive_multiplier in (1.0, 1.5):
            scores = np.zeros(len(dev_y))
            for fold in range(5):
                train = assignment != fold
                x = np.concatenate((base_x, dev_x[train]))
                y = np.concatenate((base_y, dev_y[train]))
                weights = np.ones(len(y))
                weights[y == 1] = positive_multiplier
                head = LogisticRegression(C=c, max_iter=2000, class_weight="balanced", random_state=20261019)
                head.fit(x, y, sample_weight=weights)
                scores[~train] = head.predict_proba(dev_x[~train])[:, 1]
            selected = select_threshold(dev_y, scores)
            trials.append({"C": c, "positive_multiplier": positive_multiplier, "average_precision": float(average_precision_score(dev_y, scores)), **selected})
    winner = max(trials, key=lambda row: (row["feasible"], row["precision"] if row["feasible"] else row["f1"], row["recall"], row["average_precision"]))
    weights = np.ones(len(base_y) + len(dev_y))
    full_y = np.concatenate((base_y, dev_y))
    weights[full_y == 1] = winner["positive_multiplier"]
    head = LogisticRegression(C=winner["C"], max_iter=2000, class_weight="balanced", random_state=20261019)
    head.fit(all_vectors, full_y, sample_weight=weights)
    np.savez(args.head, coef=head.coef_, intercept=head.intercept_, classes=head.classes_, threshold=winner["threshold"])
    report = {"encoder": MODEL, "revision": REVISION, "encoder_frozen": True, "embedding_dimensions": int(all_vectors.shape[1]), "head": "scikit-learn logistic regression", "base_documents": len(base_y), "AI_development_documents": len(dev_y), "cross_validation_folds": 5, "selected": winner, "trials": trials, "independent_test_read": False, "production_promotion": False, "label_authority": "historical labels plus two-model automatic development consensus"}
    args.output.write_text(json.dumps(report, indent=2) + "\n")
    print(json.dumps(report["selected"], indent=2))


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--historical", required=True, type=Path)
    parser.add_argument("--development", required=True, type=Path)
    parser.add_argument("--output", required=True, type=Path)
    parser.add_argument("--head", required=True, type=Path)
    parser.add_argument("--device", default="cpu")
    parser.add_argument("--batch-size", type=int, default=16)
    run(parser.parse_args())


if __name__ == "__main__":
    main()
