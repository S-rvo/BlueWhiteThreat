#!/usr/bin/env python3
"""docs/pistes-a-creuser-2026-11-05.md point 1: paired comparison of the
frozen production TF-IDF representation against a frozen multilingual
encoder (paraphrase-multilingual-MiniLM-L12-v2 — not EmbeddingGemma/Nomic,
see point 1's "Investigué" note for why those aren't available locally),
under identical cross-validation folds. Only the logistic regression head
is fit per fold; both representations are frozen going in.
"""
import csv
import sys

import numpy as np
from sentence_transformers import SentenceTransformer
from sklearn.linear_model import LogisticRegression
from sklearn.model_selection import StratifiedKFold
from sklearn.metrics import average_precision_score, precision_recall_fscore_support

MODEL = "sentence-transformers/paraphrase-multilingual-MiniLM-L12-v2"
SEED = 20261106
N_FOLDS = 5
RECALL_TARGETS = (0.65, 0.75, 0.85, 0.90)

csv.field_size_limit(sys.maxsize)


def load_csv(path):
    with open(path, encoding="utf-8") as f:
        rows = list(csv.DictReader(f))
    ids = [r["id"] for r in rows]
    texts = [r["text"] for r in rows]
    labels = np.array([int(r["label"]) for r in rows])
    return ids, texts, labels


def load_tfidf(prefix, n, dims=5000):
    vectors = np.fromfile(prefix + ".bin", dtype="<f8").reshape(n, dims)
    ids = [line.strip() for line in open(prefix + ".ids.txt")]
    labels = np.array([int(line.strip()) for line in open(prefix + ".labels.txt")])
    return ids, vectors, labels


def oof_scores(x, y, seed):
    folds = StratifiedKFold(n_splits=N_FOLDS, shuffle=True, random_state=seed)
    scores = np.zeros(len(y))
    for train_idx, test_idx in folds.split(x, y):
        head = LogisticRegression(max_iter=2000, class_weight="balanced", random_state=seed)
        head.fit(x[train_idx], y[train_idx])
        scores[test_idx] = head.predict_proba(x[test_idx])[:, 1]
    return scores


def min_fp_at_recall(y, scores, target):
    order = np.argsort(-scores)
    y_sorted = y[order]
    total_pos = y.sum()
    tp = fp = 0
    best_fp = None
    for label in y_sorted:
        if label == 1:
            tp += 1
        else:
            fp += 1
        if tp / total_pos >= target:
            best_fp = fp
            break
    return best_fp


def summarize(y, scores, label):
    ap = average_precision_score(y, scores)
    row = {"representation": label, "average_precision": ap}
    for target in RECALL_TARGETS:
        row[f"min_fp_at_recall_{target}"] = min_fp_at_recall(y, scores, target)
    threshold = 0.5
    predicted = scores >= threshold
    precision, recall, f1, _ = precision_recall_fscore_support(y, predicted, average="binary", zero_division=0)
    row["precision_at_0.5"] = precision
    row["recall_at_0.5"] = recall
    row["f1_at_0.5"] = f1
    return row


def main():
    reviewed_path = "data/reviewed-with-text-20261106.csv"
    tfidf_prefix = "data/point1-tfidf-vs-minilm-20261106/tfidf"

    csv_ids, texts, csv_labels = load_csv(reviewed_path)
    tfidf_ids, tfidf_x, tfidf_labels = load_tfidf(tfidf_prefix, len(csv_ids))
    if tfidf_ids != csv_ids or not np.array_equal(tfidf_labels, csv_labels):
        raise SystemExit("tfidf dump does not match reviewed CSV order/labels")

    print(f"Encoding {len(texts)} documents with {MODEL} ...", file=sys.stderr)
    encoder = SentenceTransformer(MODEL)
    minilm_x = encoder.encode(texts, batch_size=32, normalize_embeddings=True, show_progress_bar=True)

    results = []
    for seed in (SEED, SEED + 1, SEED + 2):
        tfidf_scores = oof_scores(tfidf_x, csv_labels, seed)
        minilm_scores = oof_scores(minilm_x, csv_labels, seed)
        results.append({"seed": seed, **summarize(csv_labels, tfidf_scores, "tfidf-frozen-production-vocab")})
        results.append({"seed": seed, **summarize(csv_labels, minilm_scores, "minilm-frozen-encoder")})

    for row in results:
        print(row)

    import json
    with open("data/point1-tfidf-vs-minilm-20261106/results.json", "w") as f:
        json.dump(results, f, indent=2)


if __name__ == "__main__":
    main()
