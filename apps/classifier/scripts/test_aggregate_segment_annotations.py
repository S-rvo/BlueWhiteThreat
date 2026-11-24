import hashlib
import unittest

from aggregate_segment_annotations import aggregate


class AggregateSegmentAnnotationsTests(unittest.TestCase):
    def item(self, identifier, text, label, status="consensus"):
        return {
            "id": identifier,
            "text_sha256": hashlib.sha256(text.encode()).hexdigest(),
            "label": label,
            "status": status,
        }

    def test_unanimous_complete_segments_produce_parent_consensus(self):
        docs = [
            {"id": "p:segment:1", "parent_id": "p", "text": "one"},
            {"id": "p:segment:2", "parent_id": "p", "text": "two"},
        ]
        votes = {"documents": [self.item("p:segment:1", "one", 0), self.item("p:segment:2", "two", 0)]}
        self.assertEqual(aggregate(docs, votes)["documents"][0]["status"], "consensus")
        self.assertEqual(aggregate(docs, votes)["documents"][0]["label"], 0)

    def test_mixed_or_uncertain_segments_do_not_force_parent_label(self):
        docs = [
            {"id": "p:segment:1", "parent_id": "p", "text": "one"},
            {"id": "p:segment:2", "parent_id": "p", "text": "two"},
        ]
        mixed = {"documents": [self.item("p:segment:1", "one", 0), self.item("p:segment:2", "two", 1)]}
        self.assertEqual(aggregate(docs, mixed)["documents"][0]["status"], "mixed_segments")
        uncertain = {"documents": [self.item("p:segment:1", "one", 0), self.item("p:segment:2", "two", None, "abstention")]}
        self.assertEqual(aggregate(docs, uncertain)["documents"][0]["status"], "abstention")

    def test_hash_mismatch_is_rejected(self):
        docs = [{"id": "p:segment:1", "parent_id": "p", "text": "one"}]
        votes = {"documents": [self.item("p:segment:1", "changed", 0)]}
        with self.assertRaisesRegex(ValueError, "hash mismatch"):
            aggregate(docs, votes)


if __name__ == "__main__":
    unittest.main()
