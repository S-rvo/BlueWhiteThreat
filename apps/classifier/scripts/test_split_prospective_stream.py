import unittest

from split_prospective_stream import split


class SplitProspectiveStreamTests(unittest.TestCase):
    def test_newest_documents_are_reserved(self):
        documents = [
            {"id": "b", "collected_at": "2026-01-02"},
            {"id": "a", "collected_at": "2026-01-01"},
            {"id": "c", "collected_at": "2026-01-03"},
        ]
        development, reserve = split(documents, 1)
        self.assertEqual([d["id"] for d in development], ["a", "b"])
        self.assertEqual([d["id"] for d in reserve], ["c"])

    def test_rejects_empty_side(self):
        with self.assertRaises(ValueError):
            split([{"id": "a"}], 1)


if __name__ == "__main__":
    unittest.main()
