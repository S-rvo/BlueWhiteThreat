#!/usr/bin/env python3
"""Run the bounded, synthetic pipeline against new disposable Docker databases."""

import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import secrets
import signal
import subprocess
import tempfile
import time
from urllib import error, request


ROOT = Path(__file__).resolve().parents[2]
OLLAMA_MODEL = "qwen2.5:14b-instruct"


def run(*args, **kwargs):
    timeout = kwargs.pop("timeout", 120)
    return subprocess.run(args, check=True, timeout=timeout, text=True, **kwargs)


def port(container, internal):
    value = run("docker", "port", container, f"{internal}/tcp", capture_output=True).stdout.strip()
    if not re.fullmatch(r"127\.0\.0\.1:[1-9][0-9]{0,4}", value):
        raise RuntimeError(f"expected a single loopback bind for {internal}")
    return value.split(":")[1]


def wait_db(container, *command):
    deadline = time.monotonic() + 100
    while time.monotonic() < deadline:
        try:
            run("docker", "exec", container, *command, timeout=5, capture_output=True)
            return
        except (subprocess.CalledProcessError, subprocess.TimeoutExpired):
            time.sleep(2)
    raise RuntimeError("disposable database failed readiness check")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    mode = parser.add_mutually_exclusive_group()
    mode.add_argument("--real-models", action="store_true",
                      help="use the checkout classifier and the configured Ollama model on loopback")
    mode.add_argument("--real-ollama", action="store_true",
                      help="use the fixed classifier fixture and the configured Ollama model on loopback")
    parser.add_argument("--ollama-port", type=int, default=11434, help="loopback port (default: 11434)")
    args = parser.parse_args()
    if not 1 <= args.ollama_port <= 65535:
        parser.error("--ollama-port must be between 1 and 65535")
    real_ollama = args.real_models or args.real_ollama
    # Never reuse ambient credentials, model overrides, or a service workdir.
    env = {
        key: value for key, value in os.environ.items()
        if not key.startswith(("BWT_", "MONGO_", "MONGODB_", "NEO4J_", "ANTHROPIC_", "RETENTION_"))
        and key != "RELEVANCE_THRESHOLD"
    }
    container_env = os.environ.copy()
    mongo_user = "smoke"
    mongo_password = secrets.token_hex(18)
    neo_password = secrets.token_hex(18)
    api_password = secrets.token_hex(18)
    container_env["BWT_SMOKE_API_PASSWORD"] = api_password
    container_env.update(MONGO_INITDB_ROOT_USERNAME=mongo_user,
                         MONGO_INITDB_ROOT_PASSWORD=mongo_password,
                         NEO4J_AUTH=f"neo4j/{neo_password}")
    name = f"bwt-smoke-{secrets.token_hex(6)}"
    mongo = f"{name}-mongo"
    neo = f"{name}-neo4j"
    started = []

    with tempfile.TemporaryDirectory(prefix="bwt-smoke-") as workdir:
        binary = Path(workdir) / "classifier"
        model_path = ROOT / "tests/pipeline/model.fixture.json"
        if args.real_models:
            model_bytes = (ROOT / "apps/classifier/model.json").read_bytes()
            model_path = Path(workdir) / "model.json"
            model_path.write_bytes(model_bytes)
        if real_ollama:
            model_bytes = model_path.read_bytes()
            digest = hashlib.sha256(model_bytes).hexdigest()
            version = json.loads(model_bytes).get("version") or f"legacy-{digest[:16]}"
            ollama_host = f"http://127.0.0.1:{args.ollama_port}"
            opener = request.build_opener(request.ProxyHandler({}))
            try:
                with opener.open(f"{ollama_host}/api/tags", timeout=5) as response:
                    models = json.load(response)["models"]
            except (error.URLError, TimeoutError, ValueError, KeyError) as exc:
                parser.error(f"Ollama must answer at {ollama_host}: {exc}")
            match = next((item for item in models if item["name"] == OLLAMA_MODEL), None)
            if match is None:
                parser.error(f"start local Ollama and install {OLLAMA_MODEL} before running this mode")
            print(f"Classifier {version}, sha256={digest}; Ollama {OLLAMA_MODEL}, "
                  f"digest={match['digest']}", flush=True)
            env.update(BWT_SMOKE_CLASSIFIER_VERSION=version, BWT_SMOKE_OLLAMA_MODEL=OLLAMA_MODEL,
                       BWT_SMOKE_OLLAMA_PORT=str(args.ollama_port))
        run("go", "build", "-o", str(binary), "./cmd/main", cwd=ROOT / "apps/classifier", env=env)
        try:
            for service, options in [
                (mongo, ["-p", "127.0.0.1::27017", "-e", "MONGO_INITDB_ROOT_USERNAME",
                         "-e", "MONGO_INITDB_ROOT_PASSWORD", "mongo:8.0.9"]),
                (neo, ["-p", "127.0.0.1::7687", "-e", "NEO4J_AUTH",
                       "-e", "NEO4J_server_memory_heap_initial__size=256m",
                       "-e", "NEO4J_server_memory_heap_max__size=256m",
                       "-e", "NEO4J_server_memory_pagecache_size=128m", "neo4j:5.19"]),
            ]:
                started.append(service)
                run("docker", "run", "--rm", "-d", "--name", service,
                    "--label", f"bluewhitethreat.smoke={name}", *options, env=container_env)
            wait_db(mongo, "sh", "-ec",
                    'mongosh --quiet --username "$MONGO_INITDB_ROOT_USERNAME" '
                    '--password "$MONGO_INITDB_ROOT_PASSWORD" --authenticationDatabase admin '
                    '--eval "quit(db.adminCommand({ping:1}).ok ? 0 : 1)"')
            wait_db(neo, "sh", "-ec", 'cypher-shell -u neo4j -p "${NEO4J_AUTH#neo4j/}" "RETURN 1"')

            mongo_port = port(mongo, 27017)
            mongo_uri = f"mongodb://{mongo_user}:{mongo_password}@127.0.0.1:{mongo_port}/?authSource=admin"
            env.update(
                BWT_DISPOSABLE_PIPELINE="1",
                MONGO_HOST="127.0.0.1", MONGO_PORT=mongo_port, MONGO_DB="BlueWhiteThreat",
                MONGO_INITDB_ROOT_USERNAME=mongo_user, MONGO_INITDB_ROOT_PASSWORD=mongo_password,
                MONGODB_URI=mongo_uri, BWT_PARSER_TEST_MONGO_URI=mongo_uri,
                NEO4J_URI=f"bolt://127.0.0.1:{port(neo, 7687)}", NEO4J_PASSWORD=neo_password,
                MODEL_PATH=str(model_path),
            )
            def go_test(module, package, pattern):
                run("go", "test", "-tags=integration", "-count=1", "-v", "-timeout=110s",
                    "-run", pattern, package, cwd=ROOT / "apps" / module, env=env)

            # This existing projection-retry test cleans up once its assertions finish.
            go_test("parser", "./cmd", "^TestGraphProjectionRetriesPersistedEntities$")
            go_test("scraper", "./internal/sites", "^TestSmokeIngest$")
            # Run only the scoring entrypoint, using the selected artifact and no checkout .env.
            run(str(binary), "-once", cwd=workdir, env=env)
            go_test("parser", "./cmd", "^TestSmokeExtractAndReplay$")
            if real_ollama:
                return  # The separate API contract in CI expects the fixed fixture verdicts.
            go_test("classifier", "./internal/db", "^TestSmokeRetentionReport$")
            go_test("classifier", "./cmd/export", "^TestSmokeCTIReviewExport$")
            run("docker", "exec", "-e", "BWT_SMOKE_API_PASSWORD", mongo, "sh", "-ec",
                'mongosh --quiet --username "$MONGO_INITDB_ROOT_USERNAME" '
                '--password "$MONGO_INITDB_ROOT_PASSWORD" --authenticationDatabase admin '
                "BlueWhiteThreat --eval 'db.createUser({user: \"smoke_api\", "
                "pwd: process.env.BWT_SMOKE_API_PASSWORD, roles: [\"read\"]})'",
                capture_output=True, env=container_env)
            env["MONGODB_URI"] = (
                f"mongodb://smoke_api:{api_password}@127.0.0.1:{mongo_port}/BlueWhiteThreat"
            )
            go_test("api", "./cmd", "^TestSmokeMongoContract$")
            env["RETENTION_MONGODB_URI"] = env["MONGODB_URI"]
            report = json.loads(run(str(binary), "-retention-report", cwd=workdir, env=env,
                                    capture_output=True).stdout)
            expected = {"total": 2, "keep": 1, "review": 1, "discard": 0, "pending": 0}
            if any(report[key] != count for key, count in expected.items()):
                raise RuntimeError("retention report did not match fixture verdicts")
            print("Read-only classifier retention report passed", flush=True)
        finally:
            for service in reversed(started):
                # Only touch containers carrying this invocation's unguessable label.
                result = subprocess.run(["docker", "ps", "-aq", "--filter",
                                         f"label=bluewhitethreat.smoke={name}",
                                         "--filter", f"name=^/{service}$"],
                                        capture_output=True, text=True, timeout=10)
                for container in result.stdout.split():
                    run("docker", "stop", "-t", "5", container, timeout=20)


if __name__ == "__main__":
    def interrupted(*_):
        raise KeyboardInterrupt

    signal.signal(signal.SIGTERM, interrupted)
    try:
        main()
    except subprocess.CalledProcessError as exc:
        raise SystemExit(f"Pipeline check failed ({exc.cmd[0]}, exit {exc.returncode}).") from None
