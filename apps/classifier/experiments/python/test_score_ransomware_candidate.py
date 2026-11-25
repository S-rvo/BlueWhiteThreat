import hashlib
import json
import tempfile
import unittest
from pathlib import Path

import joblib
import numpy as np

from score_ransomware_candidate import score


class FixedProbabilityModel:
    def predict_proba(self, texts):
        return np.tile([0.25, 0.75], (len(texts), 1))


class FrozenCandidateScorerTest(unittest.TestCase):
    def setUp(self):
        temp = tempfile.TemporaryDirectory()
        self.addCleanup(temp.cleanup)
        root = Path(temp.name)
        self.input = root / "input.jsonl"
        self.output = root / "output.jsonl"
        self.model = root / "model"
        self.model.mkdir()
        artifact = self.model / "candidate.joblib"
        joblib.dump(FixedProbabilityModel(), artifact)
        self.manifest_path = self.model / "manifest.json"
        self.manifest = {
            "model_sha256": hashlib.sha256(artifact.read_bytes()).hexdigest(),
            "production": False,
            "threshold": 0.5,
        }
        self.manifest_path.write_text(json.dumps(self.manifest))
        self.write_rows({"id": "d1", "text": "Synthetic document."})

    def write_rows(self, *rows):
        self.input.write_text("".join(json.dumps(row) + "\n" for row in rows))

    def test_scores_and_does_not_echo_text(self):
        self.assertEqual(score(self.input, self.output, self.model), 1)
        self.assertEqual(json.loads(self.output.read_text()), {
            "id": "d1",
            "score": 0.75,
            "threshold": 0.5,
            "prediction": 1,
            "model_sha256": self.manifest["model_sha256"],
        })

    def test_rejects_duplicate_ids(self):
        row = {"id": "d1", "text": "Synthetic document."}
        self.write_rows(row, row)
        with self.assertRaisesRegex(ValueError, "duplicate_id"):
            score(self.input, self.output, self.model)
        self.assertFalse(self.output.exists())

    def test_rejects_existing_output(self):
        self.output.write_text("existing")
        with self.assertRaises(FileExistsError):
            score(self.input, self.output, self.model)
        self.assertEqual(self.output.read_text(), "existing")

    def test_rejects_manifest_hash_mismatch(self):
        self.manifest["model_sha256"] = "0" * 64
        self.manifest_path.write_text(json.dumps(self.manifest))
        with self.assertRaisesRegex(ValueError, "candidate_hash_mismatch"):
            score(self.input, self.output, self.model)
        self.assertFalse(self.output.exists())

    def test_rejects_empty_text(self):
        self.write_rows({"id": "d1", "text": " "})
        with self.assertRaisesRegex(ValueError, "invalid_text"):
            score(self.input, self.output, self.model)
        self.assertFalse(self.output.exists())


if __name__ == "__main__":
    unittest.main()
