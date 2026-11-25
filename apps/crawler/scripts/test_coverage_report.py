import json
import os
import unittest

import coverage_report as report


CATALOG = "https://raw.githubusercontent.com/fastfire/deepdarkCTI/main/"
HOST_A = "a" * 56 + ".onion"
HOST_B = "b" * 56 + ".onion"
HOST_C = "c" * 56 + ".onion"


class CoverageTests(unittest.TestCase):
    def test_origin_matching(self):
        self.assertEqual(report.host_key(f"HTTPS://{HOST_A.upper()}:443/a"), HOST_A)
        self.assertEqual(report.host_key(f"http://{HOST_A}:8080/"), f"{HOST_A}:8080")
        for url in [f"http://user@{HOST_A}/", f"http://{HOST_A}:80@elsewhere/",
                    f"http://{HOST_A}.example/", f"ftp://{HOST_A}/", "http://a.onion/"]:
            with self.subTest(url=url):
                self.assertIsNone(report.host_key(url))

    def test_counts_provenance_and_excludes_ambiguous_hosts_from_mongo(self):
        self_case = self
        a, b1, b2, c = (f"http://{host}/{path}" for host, path in [
            (HOST_A, "first"), (HOST_B, "one"), (HOST_B, "two"), (HOST_C, "ignored")
        ])

        class Fixture:
            def redis(self, commands):
                if commands[0][0] == "SCAN":
                    cursor = commands[0][1]
                    next_cursor, urls = ("9", [a, b1]) if cursor == "0" else ("0", [a, b2, c])
                    return [[next_cursor, [report.PAGE_PREFIX + url for url in urls]]]
                if commands[0][0] == "HGET":
                    labels = {
                        a: CATALOG + "forum.md", b1: CATALOG + "forum.md",
                        b2: CATALOG + "others.md", c: CATALOG + "../ignored.md",
                    }
                    return [labels[key[len(report.PAGE_PREFIX):]] for _, key, _ in commands]
                if commands[0][1] == "crawler:last_attempt":
                    self_case.assertEqual(commands, [
                        ("HMGET", "crawler:last_attempt", a, b1, b2),
                        ("ZMSCORE", "urls_crawled_at", a, b1, b2),
                    ])
                    return [[None, "150", None], [160, None, None]]
                self_case.assertEqual(commands, [
                    ("HMGET", f"crawler:host_stats:{HOST_A}", "status_401", "status_403")
                ])
                return [[None, "3"]]

            def mongo(self, database, hosts_by_category):
                self_case.assertEqual(database, "fixture")
                self_case.assertEqual(dict(hosts_by_category), {"forum.md": [HOST_A]})
                return {"forum.md": (5, 2)}

        result = report.coverage(Fixture(), 20, "fixture")
        self.assertTrue(result["scan_complete"])
        self.assertEqual(result["scanned_page_keys"], 4)
        self.assertEqual(result["attributed_seeds"], 3)
        self.assertEqual(result["ambiguous_hosts"], 1)
        self.assertEqual(result["categories"]["forum.md"], {
            "seeds": 2, "attempted_seeds": 2, "successful_seeds": 1,
            "hosts": 1, "hosts_with_http_401_or_403": 1,
            "documents": 5, "useful_verdicts": 2,
        })
        self.assertEqual(result["categories"]["others.md"]["documents"], 0)
        for host in (HOST_A, HOST_B, HOST_C):
            self.assertNotIn(host, json.dumps(result))

    def test_cap_is_distinct_from_a_complete_empty_result(self):
        self_case = self

        class Fixture:
            def __init__(self, cursor):
                self.cursor = cursor

            def redis(self, commands):
                self_case.assertEqual(commands[0][0], "SCAN")
                return [[self.cursor, [report.PAGE_PREFIX + "x"]]]

        for cursor, complete in [("9", False), ("0", True)]:
            with self.subTest(cursor=cursor):
                result = report.coverage(Fixture(cursor), 1, "fixture")
                self.assertEqual(result["scan_complete"], complete)
                self.assertEqual(result["scanned_page_keys"], 1)
                self.assertEqual(result["categories"], {})

    @unittest.skipUnless(os.environ.get("RUN_COVERAGE_MONGO_TEST") == "1", "Compose MongoDB not requested")
    def test_mongo_pipeline_with_inline_documents(self):
        rows = [
            {"url": f"http://{HOST_A}/", "relevance": {"relevant": True, "needs_review": False}},
            {"url": f"https://{HOST_A.upper()}:443/", "relevance": {"relevant": True, "needs_review": True}},
            {"url": f"http://{HOST_A}/old", "relevance": {"relevant": True}},
            {"url": f"http://{HOST_A}/bad", "relevance": {"relevant": 1, "needs_review": False}},
            {"url": f"http://{HOST_A}:8080/"},
            {"url": f"http://{HOST_B}:8080/"},
            {"url": f"http://{HOST_B}/"},
            {"url": f"http://{HOST_A}:80@unexpected.example/", "relevance": {"relevant": True}},
            {"url": {"invalid": True}},
            {},
        ]
        constants = {
            "onionPattern": report.ONION,
            "hostsByCategory": {"forum.md": [HOST_A], "others.md": [f"{HOST_B}:8080"]},
            "fixture": rows,
        }
        script = "".join(f"const {key} = {json.dumps(value)};\n" for key, value in constants.items())
        # $documents fournit des entrées en mémoire. Ni collection créée, ni écriture.
        query = """
        print(JSON.stringify(db.aggregate(
            [{$documents: fixture}, ...coveragePipeline], {maxTimeMS: 8000}
        ).toArray()));
        """
        (result,) = report.ComposeReader()._run(
            "mongodb", ["sh", "-ec", report.MONGO_SHELL], script + report.MONGO_PIPELINE + query
        )
        self.assertEqual(sorted(result, key=lambda row: row["_id"]), [
            {"_id": "forum.md", "documents": 4, "useful_verdicts": 1},
            {"_id": "others.md", "documents": 1, "useful_verdicts": 0},
        ])


if __name__ == "__main__":
    unittest.main()
