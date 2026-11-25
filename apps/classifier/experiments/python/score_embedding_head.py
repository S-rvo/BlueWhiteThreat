#!/usr/bin/env python3
"""Score JSONL documents with a frozen encoder and saved logistic head."""

import argparse
import json
from pathlib import Path

import numpy as np
from sentence_transformers import SentenceTransformer

MODEL = "sentence-transformers/paraphrase-multilingual-MiniLM-L12-v2"
REVISION = "b8ef00830037f9868450f778081ea683e900fe39"


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--input", required=True, type=Path)
    parser.add_argument("--head", required=True, type=Path)
    parser.add_argument("--output", required=True, type=Path)
    parser.add_argument("--batch-size", type=int, default=16)
    parser.add_argument("--device", default="cpu")
    args = parser.parse_args()

    rows = [json.loads(line) for line in args.input.open(encoding="utf-8") if line.strip()]
    if len({row.get("id") for row in rows}) != len(rows) or any(not row.get("id") for row in rows):
        raise ValueError("empty or duplicate id")
    head = np.load(args.head)
    encoder = SentenceTransformer(MODEL, revision=REVISION, device=args.device)
    vectors = encoder.encode([row["text"] for row in rows], batch_size=args.batch_size, normalize_embeddings=True)
    logits = vectors @ head["coef"][0] + head["intercept"][0]
    scores = 1.0 / (1.0 + np.exp(-logits))
    threshold = float(head["threshold"])
    with args.output.open("x", encoding="utf-8") as stream:
        for row, score in zip(rows, scores):
            stream.write(json.dumps({"id": row["id"], "score": float(score), "relevant": bool(score >= threshold)}) + "\n")


if __name__ == "__main__":
    main()
