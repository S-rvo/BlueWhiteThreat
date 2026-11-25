#!/usr/bin/env python3
"""Split oversized documents at paragraph boundaries while preserving provenance."""
import argparse
import hashlib
import json
import re
from pathlib import Path


def chunks(text, target, maximum):
    paragraphs = [p.strip() for p in re.split(r"\n\s*\n|(?<=\.)\s+(?=[A-Z])", text) if p.strip()]
    out, current = [], ""
    for paragraph in paragraphs:
        while len(paragraph) > maximum:
            cut = paragraph.rfind(" ", 0, maximum)
            cut = cut if cut >= target // 2 else maximum
            if current:
                out.append(current)
                current = ""
            out.append(paragraph[:cut].strip())
            paragraph = paragraph[cut:].strip()
        candidate = f"{current}\n\n{paragraph}".strip()
        if current and len(candidate) > maximum:
            out.append(current)
            current = paragraph
        else:
            current = candidate
        if len(current) >= target:
            out.append(current)
            current = ""
    if current:
        if out and len(current) < target // 3 and len(out[-1]) + len(current) + 2 <= maximum:
            out[-1] += "\n\n" + current
        else:
            out.append(current)
    return out


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--input", type=Path, required=True)
    ap.add_argument("--out", type=Path, required=True)
    ap.add_argument("--threshold", type=int, default=12000)
    ap.add_argument("--target", type=int, default=6000)
    ap.add_argument("--maximum", type=int, default=9000)
    args = ap.parse_args()
    written = parents = 0
    with args.input.open() as src, args.out.open("x") as dst:
        for line in src:
            item = json.loads(line)
            if len(item.get("text", "")) <= args.threshold:
                continue
            parents += 1
            for index, text in enumerate(chunks(item["text"], args.target, args.maximum), 1):
                child = dict(item)
                child["parent_id"] = item["id"]
                child["id"] = f'{item["id"]}:segment:{index}'
                child["group"] = item.get("group") or item["id"]
                child["text"] = text
                child["text_sha256"] = hashlib.sha256(text.encode()).hexdigest()
                child["pool_role"] = "segmented_candidate"
                dst.write(json.dumps(child, ensure_ascii=False) + "\n")
                written += 1
    print(json.dumps({"parents": parents, "segments": written}))


if __name__ == "__main__":
    main()
