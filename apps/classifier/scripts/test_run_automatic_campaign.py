import contextlib
import io
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch
from annotate_autoeval import sha
from run_automatic_campaign import run

class ResumeCampaignTest(unittest.TestCase):
    @patch('run_automatic_campaign.verify')
    @patch('run_automatic_campaign.subprocess.run')
    def test_completed_result_does_not_reopen_test(self, process, verify):
        with tempfile.TemporaryDirectory() as td:
            p=Path(td)
            (p/'test-annotations').mkdir()
            (p/'calibration.json').write_text('{}')
            (p/'test-annotations/consensus.json').write_text('{}')
            result={'hashes':{'calibration':sha(b'{}'),'consensus':sha(b'{}')}}
            (p/'test-result.json').write_text(json.dumps(result))
            with contextlib.redirect_stdout(io.StringIO()):run(p)
            process.assert_not_called()
            (p/'test-annotations/consensus.json').write_text('{"changed":true}')
            with self.assertRaisesRegex(ValueError,'completed_test_annotations_changed'):run(p)
            process.assert_not_called()
    @patch('run_automatic_campaign.verify')
    @patch('run_automatic_campaign.subprocess.run')
    def test_interrupted_consumption_is_not_erased_or_retried(self, process, verify):
        with tempfile.TemporaryDirectory() as td:
            p=Path(td)
            (p/'test-consumption.json').write_text('{}')
            with self.assertRaisesRegex(ValueError,'test_consumed_without_completed_report'):run(p)
            process.assert_not_called()
            self.assertTrue((p/'test-consumption.json').exists())

if __name__=='__main__':unittest.main()
