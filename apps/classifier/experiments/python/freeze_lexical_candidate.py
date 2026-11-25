#!/usr/bin/env python3
"""Freeze a private Python lexical candidate and conditional group diagnostics.
No reserve is read. This is not a Go model export or a promotion command.
"""
import argparse, hashlib, json, platform
from pathlib import Path
import joblib
import numpy as np
import sklearn
from sklearn.pipeline import Pipeline, FeatureUnion
from sklearn.feature_extraction.text import TfidfVectorizer
from sklearn.linear_model import LogisticRegression
from compare_grouped_representations import metrics, select


def digest(path):return hashlib.sha256(path.read_bytes()).hexdigest()


def group_diagnostic(rows, scores, plan, thresholds, replicates=500):
    lookup={r['id']:r for r in plan['folds']};rng=np.random.default_rng(20261023);result={}
    for source in sorted(thresholds):
        ix=np.array([i for i,r in enumerate(rows) if r['source']==source and r.get('sampling_stream')!='targeted_development_enrichment'])
        labels=np.array([rows[i]['label'] for i in ix]);s=scores[ix];groups=np.array([lookup[rows[i]['id']]['group'] for i in ix]);folds=np.array([lookup[rows[i]['id']]['fold'] for i in ix]);threshold=thresholds[source]
        names,inverse=np.unique(groups,return_inverse=True);size=np.bincount(inverse);w=1/size[inverse];pred=s>=threshold
        tp=float(np.sum(w*(pred & (labels==1))));fp=float(np.sum(w*(pred & (labels==0))));fn=float(np.sum(w*(~pred & (labels==1))))
        weighted={'precision':tp/(tp+fp) if tp+fp else 0,'recall':tp/(tp+fn) if tp+fn else 0}
        boot=[]
        for _ in range(replicates):
            counts=np.bincount(rng.integers(0,len(names),len(names)),minlength=len(names));bw=counts[inverse]
            bt=float(np.sum(bw*(pred & (labels==1))));bf=float(np.sum(bw*(pred & (labels==0))));bn=float(np.sum(bw*(~pred & (labels==1))))
            boot.append([bt/(bt+bf) if bt+bf else 0,bt/(bt+bn) if bt+bn else 0])
        per_fold=[]
        for f in range(5):
            train=folds!=f;test=~train
            if not np.any(test) or not np.any(labels[train]==1):continue
            t=select(labels[train],s[train])['threshold'];per_fold.append({'fold':f,'threshold_from_other_oof_folds':t,**metrics(labels[test],s[test],t)})
        result[source]={'groups':len(names),'group_equal_weight':weighted,'conditional_group_bootstrap_95':np.quantile(boot,[.025,.975],axis=0).tolist(),'per_fold_threshold_diagnostic':per_fold}
    return {'sources':result,'replicates':replicates,'thresholds_fixed':True,'independent_evaluation':False,'limitations':'Conditional on selected OOF scores, folds and hyperparameters; bootstrap does not correct model or threshold selection. Fold threshold diagnostic is not nested CV: score-generating models overlap in training.'}


def main():
    parser=argparse.ArgumentParser();parser.add_argument('--comparison',type=Path,required=True);parser.add_argument('--out',type=Path,required=True);a=parser.parse_args();a.out.mkdir(mode=0o700)
    p=a.comparison;rows=[json.loads(l) for l in (p/'development.jsonl').read_text().splitlines()];plan=json.loads((p/'comparison-plan.json').read_text());assert digest(p/'development.jsonl')==plan['input_sha256']
    result=json.loads((p/'comparison-result.json').read_text());passing=[r for r in result['trials'] if r['all_sources_pass']]
    chosen=max(passing,key=lambda r:r['global']['f1']);assert chosen['representation']=='word_char'
    thresholds={s:r['threshold'] for s,r in chosen['by_source'].items()}
    scores=np.load(p/f"oof-word_char-{chosen['C']}.npy")
    stability=group_diagnostic(rows,scores,plan,thresholds)
    (a.out/'group-stability.json').write_text(json.dumps(stability,indent=2)+'\n')
    estimator=Pipeline([('tfidf',FeatureUnion([
        ('word',TfidfVectorizer(ngram_range=(1,2),min_df=2,max_features=20000,sublinear_tf=True)),
        ('char',TfidfVectorizer(analyzer='char',ngram_range=(3,5),min_df=2,max_features=30000,sublinear_tf=True))])),
        ('classifier',LogisticRegression(C=chosen['C'],class_weight='balanced',max_iter=2000,random_state=20261023))])
    texts=[r['text'] for r in rows];labels=[r['label'] for r in rows];estimator.fit(texts,labels)
    path=a.out/'candidate.joblib';joblib.dump({'pipeline':estimator,'source_thresholds':thresholds,'unknown_source_policy':'abstain','version':'experimental-word-char-20261023'},path)
    restored=joblib.load(path);before=estimator.predict_proba(texts)[:,1];after=restored['pipeline'].predict_proba(texts)[:,1]
    np.testing.assert_allclose(before,after,atol=1e-12,rtol=0)
    np.save(a.out/'refit-development-scores.npy',after)
    manifest={'selected':chosen,'training_documents':len(rows),'input_sha256':digest(p/'development.jsonl'),'plan_sha256':digest(p/'comparison-plan.json'),'comparison_script_sha256':digest(Path(__file__).with_name('compare_grouped_representations.py')),'export_script_sha256':digest(Path(__file__)),'candidate_sha256':digest(path),'versions':{'python':platform.python_version(),'sklearn':sklearn.__version__,'numpy':np.__version__,'joblib':joblib.__version__},'parity_documents':len(rows),'serialization_max_absolute_error':float(np.max(np.abs(before-after))),'go_inference_parity_verified':False,'deployment_runtime':'isolated Python scikit-learn pipeline; not compatible with Go model.json','refit_scores_are_training_not_evaluation':True,'reserve_read':False,'production_promotion':False,'group_stability_gate_met':all(v['group_equal_weight']['recall']>=.9 and v['group_equal_weight']['precision']>=.5 for v in stability['sources'].values())}
    (a.out/'manifest.json').write_text(json.dumps(manifest,indent=2)+'\n');print(json.dumps(manifest,indent=2),flush=True)

if __name__=='__main__':main()
