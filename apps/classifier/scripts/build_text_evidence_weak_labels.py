#!/usr/bin/env python3
"""Conservative text-only weak-label proposal; provenance is never a decision rule."""
import json,pathlib,re,hashlib,collections
BASE=pathlib.Path('apps/classifier/data');INPUT=BASE/'iteration-2026-10-23-representation-comparison/development.jsonl';OUT=BASE/'iteration-2026-10-25-source-label-audit/text-evidence-weak-labels-v10.jsonl'
POSITIVE={
 'vulnerability_mechanism':r'\bvulnerab\w*|\bCVE-20\d\d-\d+|\bGHSA-[a-z0-9-]+|\bCWE-\d+|\b(?:SQL injection|cross.site scripting|\bXSS\b|\bRCE\b|remote code execution|\bReDoS\b|authentication bypass|privilege escalation|arbitrary file read|arbitrary file write|path traversal|request smuggling|denial.of.service)\b',
 'impact_mechanism':r'\ballows?\b|\ballowing\b|\benables?\b|\bcould\b.{0,100}\b(?:attacker|unauthori[sz]ed|execute|read|write|access|crash|hang|steal|intercept)\b|\battacker\b.{0,100}\b(?:read|write|execute|access|steal|intercept|crash|hang)\b|\bunauthori[sz]ed\b.{0,80}\b(?:access|read|write|execute|disclose)\b',
 'compromise_or_exposure':r'\b(?:ransomware|ransom|extortion|exfiltrat\w*|stolen|leaked|data dump|breach|compromised credentials|account takeover|unauthori[sz]ed access|victim demand|files? (?:released|published|stolen))\b',
 'claim_event_context':r'\b(?:claims? responsibility|victim|breach|incident|compromis\w*|stolen|leaked|exfiltrat\w*|published|publication|demand|ransom|extortion|countdown)\b',
 'abbreviated_exposure_claim':r'\bnot paid\b.{0,80}\bexposed\b|\bexposed\b.{0,80}\b(?:records|files|data|addresses|phone numbers|personal information)\b|\b(?:will|going to|soon)\b.{0,80}\b(?:upload|publish|release|dump|post)\b.{0,80}\b(?:corporate|company|employee|client|customer|personal|financial)\b.{0,40}\b(?:data|records?|information|files?)\b',
 'compromised_access_offer':r'\b(?:compromised|hacked|stolen)\b.{0,60}\b(?:accounts?|credentials?|VPN|RDP|access)\b.{0,40}\b(?:for sale|offered|broker|market)\b|\b(?:accounts?|credentials?|VPN|RDP|access)\b.{0,40}\b(?:for sale|offered by access broker)\b'
}
NEGATIVE={
 'ordinary_setup_troubleshooting':r'\b(?:troubleshooting|setup wizard|install(?:ation)? guide|how to install|account settings|configure your email account)\b',
 'ordinary_personal_productivity':r'\b(?:email client|printer setup|password settings|software installation|translation update|button alignment)\b',
 'prepaid_or_high_balance_offer':r'\b(?:prepaid cards?|high balances?|top.?up)\b',
 'generic_crypto_or_payment_offer':r'\b(?:cryptocurrency payment|crypto(?:currency)? accepted|payment methods?)\b',
 'due_diligence_or_background_service':r'\b(?:due diligence|background reports?|client investigation)\b',
 'ordinary_professional_service':r'\bconsulting services?\b',
 'company_profile_context':r'\b(?:company profile|business profile)\b',
 'business_history_or_location':r'\b(?:founded|headquartered|based in|operates in|provides services|industry leader)\b',
 'generic_product_listing':r'\b(?:products?|prices?|product catalog|quantities)\b',
 'ordinary_ecommerce_context':r'\b(?:orders?|shipping options|shopping cart|add to cart|retail store)\b',
 'directory_navigation':r'\b(?:directory|link index|category listing|site index|navigation links)\b',
 'neutral_privacy_or_software_guide':r'\b(?:privacy guide|free software|gnupg|email client|software installation)\b'
}
POTENTIAL_CTI=r'\b(?:credential|password|account|access|RDP|VPN|malware|exploit|payload|botnet|ransomware|virus|trojan|database|customer records?|stolen|leak|dump|expos\w*|payment card|credit card|bank card|hitman|killers?|assassination|murder|counterfeit|forged|fake IDs?|weapons?|drugs?|cloned cards?|carding)\b'
NEGATED_CTI=r'\b(?:does not|doesn.t|do not|don.t|did not|didn.t|could not|couldn.t|cannot|can.t|isn.t|is not|aren.t|are not|never|no|without|denies|denied|disputes|disputed|false|alleged|unconfirmed|unverified)\b.{0,100}\b(?:vulnerab\w*|allow\w*|attacker|breach|hacked|stolen|leak\w*|ransomware|compromis\w*|exfiltrat\w*|account takeover|unauthori[sz]ed access|files?|data|claim\w*)\b'
UNCERTAIN_CONTEXT=r'\b(?:alleged|potential|hypothetical|fictional|tabletop|simulation|simulated|under investigation|unconfirmed|unverified|no conclusion|incident response|ransomware recovery|how ransomware works|show all|recommended|category listing|listing active leak sites|buy and sell|marketplace directory|AI generated|I don.t have (?:any )?reliable information|without access to|appears to be an anonymized|possibly from a ransomware leak site|coded identifier)\b'
# no source decision; these cues only block a weak negative when the text is ambiguous

def classify(text):
 hits={k:bool(re.search(p,text,re.I|re.S)) for k,p in POSITIVE.items()}; neg={k:bool(re.search(p,text,re.I|re.S)) for k,p in NEGATIVE.items()}
 vulnerability=hits['vulnerability_mechanism'] and hits['impact_mechanism']; incident=(hits['compromise_or_exposure'] and hits['claim_event_context']) or hits['abbreviated_exposure_claim'] or hits['compromised_access_offer']
 if (vulnerability or incident) and re.search(UNCERTAIN_CONTEXT,text,re.I|re.S):
  return {'candidate':'abstain','rule':'hypothetical_or_non-event_context','matched':{**{k:v for k,v in hits.items() if v},'uncertain_context':True}}
 if (vulnerability or incident) and re.search(NEGATED_CTI,text,re.I|re.S):
  return {'candidate':'abstain','rule':'negated_or_disputed_cti_evidence','matched':{**{k:v for k,v in hits.items() if v},'negation':True}}
 if vulnerability or incident:return {'candidate':'weak_positive','rule':'explicit_vulnerability_and_impact' if vulnerability else 'explicit_compromise_and_claim_context','matched':{k:v for k,v in hits.items() if v}}
 if sum(neg.values())>=2 and not any(hits.values()) and not re.search(POTENTIAL_CTI,text,re.I):return {'candidate':'weak_negative','rule':'two_ordinary_content_signals_and_no_cti_markers','matched':{k:v for k,v in neg.items() if v}}
 return {'candidate':'abstain','rule':'insufficient_or_mixed_text_evidence','matched':{k:v for k,v in {**hits,**neg}.items() if v}}

def main():
 rows=[json.loads(x) for x in INPUT.read_text().splitlines() if x.strip()];out=[];counts=collections.defaultdict(collections.Counter)
 for r in rows:
  d=classify(r['text']);out.append({'id':r['id'],'source':r['source'],'text_sha256':hashlib.sha256(r['text'].encode()).hexdigest(),'historical_label':r['label'],'candidate':d['candidate'],'rule':d['rule'],'matched':d['matched']});counts[r['source']][d['candidate']]+=1;counts[r['source']]['old_label_changed_candidate']+=int(d['candidate'] in ('weak_positive','weak_negative') and int(d['candidate']=='weak_positive')!=r['label'])
 assert not OUT.exists();OUT.write_text(''.join(json.dumps(x)+'\n' for x in out));report={'input_sha256':hashlib.sha256(INPUT.read_bytes()).hexdigest(),'output_sha256':hashlib.sha256(OUT.read_bytes()).hexdigest(),'documents':len(rows),'rules':{'positive':POSITIVE,'negative':NEGATIVE,'potential_cti_veto':POTENTIAL_CTI,'negation_veto':NEGATED_CTI,'uncertain_context_veto':UNCERTAIN_CONTEXT},'counts_by_source':{s:dict(c) for s,c in counts.items()},'labels_changed':False,'used_for_training':False,'rule':'text only; source is never a decision feature; uncertainty abstains','warning':'deterministic weak-label proposal; regex misses paraphrases and may match negation/context; not ground truth'};(OUT.parent/'text-evidence-weak-label-summary-v10.json').write_text(json.dumps(report,indent=2)+'\n');print(json.dumps(report['counts_by_source'],indent=2))
if __name__=='__main__':main()
