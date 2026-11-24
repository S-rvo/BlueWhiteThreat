import unittest
from annotation_v3 import POLICY, consensus, request_for, segments, validate


class StructuredAnnotationV3Test(unittest.TestCase):
    def test_segments_and_evidence_are_verbatim(self):
        text = "Écran 😀 " + ("full dump offered for sale; " * 40)
        parts = segments(text)
        self.assertEqual("".join(part["text"] for part in parts), text)
        vote = validate({"label": 1, "reason": "A server dump is offered for sale.", "evidence_ids": [0]}, parts)
        self.assertEqual(vote["evidence"][0]["text"], parts[0]["text"])

    def test_policy_makes_compromised_data_explicit(self):
        for phrase in ("full server dump for sale", "upload corporate data soon", "countdown to publication after a security breach"):
            self.assertIn(phrase, POLICY)
        request, _ = request_for({"text": "sample"}, "qwen3:8b")
        self.assertNotIn("score", request["prompt"].lower())
        self.assertFalse(request["think"])

    def test_disagreement_stays_unlabelled(self):
        docs = [{"id": "x", "text": "release countdown"}]
        votes = {("x", "a"): {"status": "decided", "label": 1}, ("x", "b"): {"status": "decided", "label": 0}}
        result = consensus(docs, votes, ["a", "b"])["documents"][0]
        self.assertEqual(result["status"], "disagreement")
        self.assertIsNone(result["label"])


if __name__ == "__main__":
    unittest.main()
