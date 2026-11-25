"""Report on retained DeepDarkCTI provenance without contacting catalogued sites."""

import argparse
import json
import re
import subprocess
import sys
import time
from collections import defaultdict
from datetime import datetime, timezone
from pathlib import Path


ROOT = Path(__file__).resolve().parents[3]
PAGE_PREFIX = "crawler:page:"
SOURCE = re.compile(
    r"https://raw\.githubusercontent\.com/fastfire/deepdarkCTI/main/([a-z][a-z0-9_]{0,59}\.md)"
)
ONION = r"^https?://([a-z2-7]{56}\.onion)(?::([0-9]{1,5}))?(?=[/?#]|$)"
ONION_URL = re.compile(ONION, re.ASCII | re.IGNORECASE)
MAX_PAGE_KEYS = 100_000
BATCH = 256
SCAN_BATCH = 2048
MONGO_SHELL = (
    'mongosh --quiet --host 127.0.0.1 --username "$MONGO_INITDB_ROOT_USERNAME" '
    '--password "$MONGO_INITDB_ROOT_PASSWORD" --authenticationDatabase admin --file /dev/stdin'
)
MONGO_PIPELINE = """
const origin = {
    $regexFind: {
        input: {$cond: [{$eq: [{$type: "$url"}, "string"]}, "$url", ""]},
        regex: onionPattern,
        options: "i"
    }
};
const source = {
    $let: {
        vars: {parts: origin},
        in: {$cond: [
            {$eq: ["$$parts", null]}, null,
            {$let: {
                vars: {
                    host: {$toLower: {$arrayElemAt: ["$$parts.captures", 0]}},
                    port: {$arrayElemAt: ["$$parts.captures", 1]}
                },
                in: {$cond: [
                    {$in: ["$$port", [null, "80", "443"]]},
                    "$$host",
                    {$concat: ["$$host", ":", "$$port"]}
                ]}
            }}
        ]}
    }
};
const coveragePipeline = [
    {$project: {
        _id: 0, host: source,
        relevant: "$relevance.relevant", review: "$relevance.needs_review"
    }},
    {$set: {category: {$switch: {
        branches: Object.entries(hostsByCategory).map(([category, hosts]) => ({
            case: {$in: ["$host", hosts]}, then: category
        })),
        default: null
    }}}},
    {$match: {category: {$ne: null}}},
    {$group: {
        _id: "$category",
        documents: {$sum: 1},
        useful_verdicts: {$sum: {$cond: [
            {$and: [{$eq: ["$relevant", true]}, {$eq: ["$review", false]}]}, 1, 0
        ]}}
    }}
];
"""
MONGO_QUERY = """
print(JSON.stringify(db.getSiblingDB(dbName).article.aggregate(
    coveragePipeline, {maxTimeMS: 8000, allowDiskUse: false}
).toArray()));
"""


class ReportError(Exception):
    pass


class ComposeReader:
    def __init__(self):
        self.deadline = time.monotonic() + 60

    def _run(self, service, args, payload):
        try:
            timeout = self.deadline - time.monotonic()
            if timeout <= 0:
                raise subprocess.TimeoutExpired(service, 60)
            proc = subprocess.run(
                ["docker", "compose", "exec", "-T", service, *args],
                cwd=ROOT,
                input=payload,
                text=True,
                encoding="utf-8",
                capture_output=True,
                check=True,
                timeout=timeout,
            )
            return [json.loads(line) for line in proc.stdout.splitlines() if line.strip()]
        except subprocess.TimeoutExpired:
            raise ReportError("Délai du rapport dépassé ; aucun bilan publié.") from None
        except (OSError, ValueError, subprocess.CalledProcessError):
            raise ReportError(f"Lecture de {service} en échec ; aucun bilan publié.") from None

    def redis(self, commands):
        # Arguments imprimables seulement : Redis CLI comprend les guillemets
        # et échappements ASCII de JSON sans passer par un shell.
        payload = "".join(" ".join(map(json.dumps, command)) + "\n" for command in commands)
        result = self._run("redis", ["redis-cli", "-3", "--json"], payload)
        if len(result) != len(commands):
            raise ReportError("Réponse Redis incomplète ; aucun bilan publié.")
        return result

    def mongo(self, database, hosts_by_category):
        values = {
            "dbName": database,
            "onionPattern": ONION,
            "hostsByCategory": hosts_by_category,
        }
        script = "".join(f"const {name} = {json.dumps(value)};\n" for name, value in values.items())
        result = self._run("mongodb", ["sh", "-ec", MONGO_SHELL], script + MONGO_PIPELINE + MONGO_QUERY)
        if len(result) != 1 or not isinstance(result[0], list):
            raise ReportError("Réponse MongoDB incomplète ; aucun bilan publié.")
        # Mongo renvoie des agrégats seulement, jamais de documents ni d'URL.
        return {row["_id"]: (row["documents"], row["useful_verdicts"]) for row in result[0]}


def chunks(items, size=BATCH):
    for i in range(0, len(items), size):
        yield items[i:i + size]


def host_key(url):
    match = ONION_URL.match(url)
    if match is None:
        return None
    host, port = match.groups()
    return host.lower() if port in (None, "80", "443") else f"{host.lower()}:{port}"


def stored_seeds(redis, max_page_keys):
    seen = set()
    seeds = []
    cursor = "0"
    while True:
        (cursor, keys), = redis([("SCAN", cursor, "MATCH", PAGE_PREFIX + "*", "COUNT", str(SCAN_BATCH))])
        candidates = []
        truncated = False
        for key in keys:
            if key in seen:
                continue
            if len(seen) == max_page_keys:
                truncated = True
                break
            seen.add(key)
            if not key.isascii() or not key.isprintable():
                continue
            url = key[len(PAGE_PREFIX):]
            host = host_key(url)
            if host is not None:
                candidates.append((key, url, host))

        for batch in chunks(candidates, SCAN_BATCH):
            labels = redis([("HGET", key, "seed_source") for key, _, _ in batch])
            for (_, url, host), label in zip(batch, labels, strict=True):
                match = SOURCE.fullmatch(label or "")
                if match:
                    seeds.append((url, host, match[1]))

        if truncated or (str(cursor) != "0" and len(seen) == max_page_keys):
            return seeds, len(seen), False
        if str(cursor) == "0":
            return seeds, len(seen), True


def empty_metrics():
    return {
        "seeds": 0, "attempted_seeds": 0, "successful_seeds": 0,
        "hosts": 0, "hosts_with_http_401_or_403": 0,
        "documents": 0, "useful_verdicts": 0,
    }


def coverage(reader, max_page_keys, database):
    seeds, scanned, complete = stored_seeds(reader.redis, max_page_keys)
    metrics = defaultdict(empty_metrics)
    sources_by_host = defaultdict(set)
    for batch in chunks(seeds):
        urls = [url for url, _, _ in batch]
        attempts, successes = reader.redis([
            ("HMGET", "crawler:last_attempt", *urls),
            ("ZMSCORE", "urls_crawled_at", *urls),
        ])
        for (_, host, category), attempt, success in zip(batch, attempts, successes, strict=True):
            sources_by_host[host].add(category)
            row = metrics[category]
            row["seeds"] += 1
            row["attempted_seeds"] += attempt is not None or success is not None
            row["successful_seeds"] += success is not None

    categories = defaultdict(list)
    owners = [(host, next(iter(labels))) for host, labels in sources_by_host.items() if len(labels) == 1]
    for batch in chunks(owners):
        statuses = reader.redis([
            ("HMGET", "crawler:host_stats:" + host, "status_401", "status_403")
            for host, _ in batch
        ])
        for (host, category), (unauthorized, forbidden) in zip(batch, statuses, strict=True):
            categories[category].append(host)
            metrics[category]["hosts"] += 1
            metrics[category]["hosts_with_http_401_or_403"] += bool(
                int(unauthorized or 0) or int(forbidden or 0)
            )

    if categories:
        for category, (documents, useful) in reader.mongo(database, categories).items():
            if category not in categories:
                raise ReportError("Catégorie MongoDB inattendue ; aucun bilan publié.")
            metrics[category]["documents"] = documents
            metrics[category]["useful_verdicts"] = useful

    return {
        "source": "DeepDarkCTI",
        "observed_at": datetime.now(timezone.utc).isoformat(timespec="seconds"),
        "scan_complete": complete,
        "scanned_page_keys": scanned,
        "attributed_seeds": len(seeds),
        "ambiguous_hosts": len(sources_by_host) - len(owners),
        "categories": dict(sorted(metrics.items())),
    }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--max-page-keys", type=int, default=MAX_PAGE_KEYS)
    parser.add_argument("--mongo-db", default="BlueWhiteThreat")
    args = parser.parse_args()
    if not 1 <= args.max_page_keys <= MAX_PAGE_KEYS:
        parser.error(f"--max-page-keys doit être compris entre 1 et {MAX_PAGE_KEYS}")
    try:
        report = coverage(ComposeReader(), args.max_page_keys, args.mongo_db)
    except ReportError as exc:
        print(exc, file=sys.stderr)
        return 1
    except (ValueError, TypeError, KeyError):
        print("Métadonnées inattendues ; aucun bilan publié.", file=sys.stderr)
        return 1
    print(json.dumps(report, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
