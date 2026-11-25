import unittest
from automatic_campaign import metrics, wilson, near, group_support, groups_supported

class CampaignTest(unittest.TestCase):
    def test_confusion_matrix_and_exact_threshold_boundary(self):
        rows=[dict(label=1,score=.7),dict(label=1,score=.4),dict(label=0,score=.6),dict(label=0,score=.2)]
        m=metrics(rows,.6)
        self.assertEqual((m['tp'],m['fn'],m['fp'],m['tn']),(1,1,1,1))
        self.assertEqual((m['precision'],m['recall'],m['f1']),(.5,.5,.5))
        self.assertEqual(m['transmission_rate'],.5)
    def test_undefined_metrics_are_not_success(self):
        m=metrics([], .5)
        self.assertIsNone(m['precision'])
        self.assertIsNone(m['recall'])
        self.assertIsNone(m['precision_ci95'])
    def test_small_samples_keep_wide_uncertainty(self):
        lo,hi=wilson(1,1)
        self.assertLess(lo,.21)
        self.assertAlmostEqual(hi,1)
        lo,hi=wilson(0,1)
        self.assertAlmostEqual(lo,0)
        self.assertGreater(hi,.79)
    def test_duplicate_and_simhash_boundary(self):
        a=dict(id='a',normalized='one',fingerprint='0000000000000000')
        b=dict(id='b',normalized='two',fingerprint='00000000000003ff')
        self.assertTrue(near(a,b))
        b['fingerprint']='00000000000007ff'
        self.assertFalse(near(a,b))
    def test_stratum_support_blocks_source_label_confounding(self):
        # One stratum per class can produce strong aggregate metrics while
        # providing no within-source evidence; reject it for qualification.
        rows=([dict(stratum='security',label=1) for _ in range(50)] +
              [dict(stratum='general',label=0) for _ in range(50)])
        self.assertFalse(groups_supported(rows,'stratum'))
        self.assertEqual(group_support(rows,'stratum'), {
            'general':{'positive':0,'negative':50,'passed':False},
            'security':{'positive':50,'negative':0,'passed':False}})
    def test_stratum_support_passes_only_with_both_classes(self):
        rows=([dict(stratum='security',label=1) for _ in range(10)] +
              [dict(stratum='security',label=0) for _ in range(10)] +
              [dict(stratum='general',label=1) for _ in range(12)] +
              [dict(stratum='general',label=0) for _ in range(11)])
        self.assertTrue(groups_supported(rows,'stratum'))
    def test_source_support_detects_source_label_confounding(self):
        rows=([dict(source='CISA',label=1) for _ in range(25)] +
              [dict(source='Q&A',label=0) for _ in range(25)])
        self.assertFalse(groups_supported(rows,'source'))

if __name__=='__main__':unittest.main()

class FrozenExecutableTest(unittest.TestCase):
    def test_frozen_inference_binary_keeps_executable_permission(self):
        import csv,json,stat,tempfile
        from pathlib import Path
        from types import SimpleNamespace
        from unittest.mock import patch
        from automatic_campaign import freeze
        import contextlib,io
        with tempfile.TemporaryDirectory() as td:
            root=Path(td)/'apps/classifier';root.mkdir(parents=True)
            (root/'data').mkdir()
            (Path(td)/'docs/validation').mkdir(parents=True)
            (Path(td)/'docs/validation/nlp-synthetic-fixtures-2026-10-19.json').write_text('[]')
            for name in ['training_data.csv','data/reviewed_labels.csv']:
                (root/name).write_text('id,text,label\nold,old topic,0\n')
            (root/'model.json').write_text('{}')
            dev=root/'development.jsonl';dev.write_text('{"id":"dev","text":"development"}\n')
            binary=root/'inference';binary.write_text('#!/bin/sh\nexit 0\n');binary.chmod(0o700)
            pool=root/'pool.jsonl'
            with pool.open('w') as out:
                for i in range(400):
                    out.write(json.dumps(dict(id=str(i),text='text '+str(i),group=str(i),stratum='en_security' if i<200 else 'en_general',source='alpha' if i<200 else 'beta',source_type='question',language='en',url='https://example.test/'+str(i),collected_at='2026-10-19'))+'\n')
            args=SimpleNamespace(dir=root/'frozen',development=dev,binary=binary,pool=[pool])
            def prints(_,docs):return {d['id']:{'id':d['id'],'normalized':d['text'],'fingerprint':'0'} for d in docs}
            with patch('automatic_campaign.ROOT',root),patch('automatic_campaign.fingerprints',side_effect=prints),patch('automatic_campaign.near',return_value=False),contextlib.redirect_stdout(io.StringIO()):
                freeze(args)
            self.assertEqual((args.dir/'evaldocuments').stat().st_mode & 0o777,0o700)
            for split in ('validation','test'):
                counts={}
                for line in (args.dir/(split+'.jsonl')).read_text().splitlines():
                    source=json.loads(line)['source']
                    counts[source]=counts.get(source,0)+1
                self.assertEqual(counts,{'alpha':100,'beta':100})
            with self.assertRaisesRegex(ValueError,'campaign_directory_already_exists'):freeze(args)
