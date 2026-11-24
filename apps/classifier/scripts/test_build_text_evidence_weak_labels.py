import unittest
from build_text_evidence_weak_labels import classify

class ConservativeWeakLabelTest(unittest.TestCase):
 def test_concrete_vulnerability_and_impact_positive(self):
  self.assertEqual(classify('A path traversal vulnerability allows an attacker to read arbitrary files.')['candidate'],'weak_positive')
 def test_compromise_claim_positive(self):
  self.assertEqual(classify('The group claims it stole data from the victim and will publish it unless the ransom is paid.')['candidate'],'weak_positive')
 def test_source_or_single_security_word_not_enough(self):
  self.assertEqual(classify('Security update information.')['candidate'],'abstain')
 def test_one_generic_setup_signal_abstains(self):
  self.assertEqual(classify('Troubleshooting this email account.')['candidate'],'abstain')
 def test_ordinary_setup_pair_negative(self):
  self.assertEqual(classify('Troubleshooting: use the setup wizard to install your email client and configure your email account.')['candidate'],'abstain')
 def test_generic_financial_offer_negative(self):
  self.assertEqual(classify('Prepaid cards with high balances, top-up and cryptocurrency payment methods are available.')['candidate'],'weak_negative')
 def test_exposed_claim_overrides_business_service_negative(self):
  self.assertEqual(classify('Client investigation reports with names and addresses. Not paid — exposed.')['candidate'],'weak_positive')
 def test_company_profile_requires_multiple_signals(self):
  self.assertEqual(classify('A company profile.')['candidate'],'abstain')
  self.assertEqual(classify('Company profile: founded in 2001, headquartered in Paris, provides services across the industry.')['candidate'],'weak_negative')
 def test_generic_product_listing_negative(self):
  self.assertEqual(classify('Product catalog with prices, orders, shipping options and quantities.')['candidate'],'weak_negative')
 def test_compromised_access_offer_positive(self):
  self.assertEqual(classify('Compromised RDP accounts for sale through an access broker.')['candidate'],'weak_positive')
 def test_cti_terms_veto_weak_negative(self):
  self.assertEqual(classify('Account passwords, prepaid cards, top-up and cryptocurrency payment methods are available.')['candidate'],'abstain')
 def test_negated_vulnerability_abstains(self):
  self.assertEqual(classify('This vulnerability does not allow an attacker to read files.')['candidate'],'abstain')
 def test_disputed_incident_abstains(self):
  self.assertEqual(classify('The company denies ransomware claims and says no files were leaked.')['candidate'],'abstain')
 def test_unpaid_exposure_is_not_misread_as_negation(self):
  self.assertEqual(classify('Not paid — exposed customer records.')['candidate'],'weak_positive')
 def test_claim_denial_abstains(self):
  self.assertEqual(classify('They do not claim responsibility for a breach.')['candidate'],'abstain')
 def test_alleged_event_abstains(self):
  self.assertEqual(classify('The alleged breach exposed customer records.')['candidate'],'abstain')
 def test_fictional_incident_abstains(self):
  self.assertEqual(classify('The report discusses a breach and stolen files, but it is a fictional tabletop scenario.')['candidate'],'abstain')
 def test_incident_response_service_abstains(self):
  self.assertEqual(classify('We provide consulting services for incident response and ransomware recovery.')['candidate'],'abstain')
 def test_investigation_status_abstains(self):
  self.assertEqual(classify('Potential breach claims are under investigation; no conclusion yet.')['candidate'],'abstain')
 def test_marketplace_index_abstains(self):
  self.assertEqual(classify('Dark web marketplace category: ransomware, leaked documents, recommended; show all.')['candidate'],'abstain')
 def test_explicit_denial_of_stolen_goods_abstains(self):
  self.assertEqual(classify("Our gift cards aren't hacked or stolen or associated with criminal activities.")['candidate'],'abstain')
 def test_direct_victim_claim_still_positive(self):
  self.assertEqual(classify('Attackers stole the victim files; the group claims responsibility for the breach.')['candidate'],'weak_positive')
 def test_illicit_market_listing_is_not_ordinary_negative(self):
  self.assertEqual(classify('Hitman services with prices, products, orders and shipping options.')['candidate'],'abstain')
  self.assertEqual(classify('Cloned credit cards and counterfeit IDs in a product catalog with prices and quantities.')['candidate'],'abstain')
 def test_future_publication_of_sensitive_corporate_data_positive(self):
  self.assertEqual(classify('We will upload 9gb of corporate data soon, including employee personal information.')['candidate'],'weak_positive')
 def test_ordinary_file_upload_guide_not_positive(self):
  self.assertEqual(classify('Troubleshooting: use the setup wizard to upload a file and configure your email account.')['candidate'],'abstain')
 def test_ai_generated_speculation_abstains(self):
  self.assertEqual(classify("Paid Victim [AI generated] N/A. I don't have any reliable information; this appears to be an anonymized identifier, possibly from a ransomware leak site.")['candidate'],'abstain')
if __name__=='__main__':unittest.main()
