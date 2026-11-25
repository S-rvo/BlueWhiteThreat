#!/usr/bin/env python3
"""Development-only ablation: common provenance/content folds, no reserve input."""
import argparse, hashlib, json, os
from pathlib import Path
import numpy as np
from sklearn.model_selection import StratifiedGroupKFold
from sklearn.feature_extraction.text import TfidfVectorizer
from sklearn.pipeline import FeatureUnion
from sklearn.linear_model import LogisticRegression
from sklearn.metrics import average_precision_score, precision_recall_curve


def metrics(y, scores, threshold):
    pred=scores>=threshold
    tp=int(np.sum(pred & (y==1)));fp=int(np.sum(pred & (y==0)))
    fn=int(np.sum(~pred & (y==1)));tn=int(np.sum(~pred & (y==0)))
    p=tp/(tp+fp) if tp+fp else 0;r=tp/(tp+fn) if tp+fn else 0
    return dict(precision=p,recall=r,f1=2*p*r/(p+r) if p+r else 0,tp=tp,fp=fp,fn=fn,tn=tn)


def select(y,scores):
    precision,recall,thresholds=precision_recall_curve(y,scores)
    eligible=np.flatnonzero(recall[:-1]>=.9)
    i=max(eligible,key=lambda i:(precision[i],thresholds[i]))
    return dict(threshold=float(thresholds[i]),**metrics(y,scores,thresholds[i]))


def main():
    parser=argparse.ArgumentParser();parser.add_argument('--directory',type=Path,required=True);a=parser.parse_args();p=a.directory
    raw=(p/'development.jsonl').read_bytes();rows=[json.loads(l) for l in raw.splitlines() if l]
    ids=[r['id'] for r in rows];texts=[r['text'] for r in rows];y=np.array([r['label'] for r in rows]);src=np.array([r['source'] for r in rows])
    parent=list(range(len(rows)))
    def root(i):
        while parent[i]!=i:parent[i]=parent[parent[i]];i=parent[i]
        return i
    seen={}
    for i,r in enumerate(rows):
        provenance=r.get('group','')
        if r['source']=='ransomware.live' and r.get('source_actor'):provenance='ransomware-actor:'+r['source_actor'].strip().lower()
        for key in (r['content_component'],'provenance:'+provenance if provenance else ''):
            if not key:continue
            if key in seen:parent[root(i)]=root(seen[key])
            else:seen[key]=i
    groups=np.array([root(i) for i in range(len(rows))]);folds=np.zeros(len(rows),dtype=int)
    strata=np.array([r['source']+':'+str(r['label']) for r in rows])
    for fold,(_,test) in enumerate(StratifiedGroupKFold(5,shuffle=True,random_state=20261023).split(texts,strata,groups)):folds[test]=fold
    for g in set(groups):assert len(set(folds[groups==g]))==1
    plan={'input_sha256':hashlib.sha256(raw).hexdigest(),'documents':len(rows),'folds':[{'id':id,'fold':int(f),'group':int(g)} for id,f,g in zip(ids,folds,groups)],'C':[.1,1,10],'training':'development-only; no historical base, fair comparison among these experiments, not directly comparable to iteration28','encoder_revision':'b8ef00830037f9868450f778081ea683e900fe39','embedding_pooling':'mean over every consecutive 100-word chunk; encoder frozen; token truncation remains possible within a chunk','reserve_read':False,'production_promotion':False}
    planpath=p/'comparison-plan.json'
    if planpath.exists():assert json.loads(planpath.read_text())==plan
    else:planpath.write_text(json.dumps(plan,indent=2)+'\n')
    representative=np.array([r.get('sampling_stream')!='targeted_development_enrichment' for r in rows])
    results=[]
    def evaluate(name,build):
        scores={c:np.zeros(len(y)) for c in plan['C']}
        for f in range(5):
            train=folds!=f;test=~train;xtrain,xtest=build(train,test)
            for c in plan['C']:
                model=LogisticRegression(C=c,class_weight='balanced',max_iter=2000,random_state=20261023)
                model.fit(xtrain,y[train]);scores[c][test]=model.predict_proba(xtest)[:,1]
            print(name,'fold',f+1,flush=True)
        for c,s in scores.items():
            by={};pred=np.zeros(len(y),dtype=bool)
            for source in sorted(set(src)):
                ix=representative & (src==source);by[source]=select(y[ix],s[ix]);pred[src==source]=s[src==source]>=by[source]['threshold']
            ix=representative
            result={'representation':name,'C':c,'average_precision':float(average_precision_score(y[ix],s[ix])),'by_source':by,'global':metrics(y[ix],pred[ix].astype(float),.5),'all_sources_pass':all(v['precision']>=.5 and v['recall']>=.9 for v in by.values())}
            results.append(result)
            np.save(p/f'oof-{name}-{c}.npy',s)
        (p/'comparison-result.json').write_text(json.dumps({'trials':results,'independent_test':False,'production_promotion':False},indent=2)+'\n')
    def lexical(kind):
        def build(train,test):
            word=TfidfVectorizer(ngram_range=(1,2),min_df=2,max_features=20000,sublinear_tf=True)
            char=TfidfVectorizer(analyzer='char',ngram_range=(3,5),min_df=2,max_features=30000,sublinear_tf=True)
            v={'word':word,'char':char,'word_char':FeatureUnion([('word',word),('char',char)])}[kind]
            return v.fit_transform([t for t,b in zip(texts,train) if b]),v.transform([t for t,b in zip(texts,test) if b])
        return build
    for kind in ['word','char','word_char']:evaluate(kind,lexical(kind))
    from sentence_transformers import SentenceTransformer
    encoder=SentenceTransformer('sentence-transformers/paraphrase-multilingual-MiniLM-L12-v2',revision=plan['encoder_revision'],local_files_only=True,device='cpu')
    vectors=[]
    for i,text in enumerate(texts):
        words=text.split();chunks=[' '.join(words[j:j+100]) for j in range(0,len(words),100)] or ['']
        v=encoder.encode(chunks,batch_size=32,normalize_embeddings=True,show_progress_bar=False).mean(axis=0);v/=max(np.linalg.norm(v),1e-12);vectors.append(v)
        if i%100==0:print('embedding',i,'/',len(texts),flush=True)
    x=np.array(vectors);np.save(p/'embeddings.npy',x);evaluate('minilm',lambda train,test:(x[train],x[test]))
    print('COMPLETED',flush=True)

if __name__=='__main__':main()
