# Classifier : registre des essais

Lire ce registre avant de modifier le modèle. Il indexe les essais retrouvés
dans les rapports, y compris les échecs, les tests consommés et les pistes
non vérifiées. Il ne remplace pas les rapports : lire le protocole et ses
limites avant de comparer deux chiffres.

## Par où reprendre

1. Conserver le modèle en place comme référence. Les scores ci-dessous sont
   historiques, pas une mesure du modèle ou du trafic d’aujourd’hui.
2. Voir d’abord les [tests de la pipeline réelle](#tests-sur-la-pipeline-réelle)
   et le [diagnostic des faux positifs](../../docs/validation/nlp-why-no-improvement-2026-10-30.md).
   Les gains sur un corpus équilibré, public, ciblé ou annoté par LLM ne
   prouvent pas un gain en production.
3. Ne pas rouvrir un test consommé pour régler un seuil, choisir des
   caractéristiques ou enrichir l’entraînement. Toute nouvelle qualification
   doit être gelée, indépendante dans le temps et séparée par groupes
   (hôtes/acteurs/quasi-doublons). Mesurer couverture, effectifs, précision,
   rappel et faux positifs à rappel comparable. Identifier l’autorité des labels.

## Tests sur la pipeline réelle

| Essai | Résultat constaté | Statut |
| --- | --- | --- |
| [19 oct. : répétitions synthétiques et pipeline](../../docs/validation/2026-10-19-pipeline-nlp.md) | Vérifications de stabilité et de compatibilité des modèles, avant/après correction. | Régression logicielle, pas une mesure de qualité terrain. |
| [29 oct. : lot rétrospectif](../../docs/validation/nlp-actual-pipeline-holdout-2026-10-29.md) | 1 512 consensus LLM. Production : précision 32,68 %, rappel 92,77 % ; candidat ransomware.live : 22,09 % / 97,87 %. | Test consommé ; les deux échouent en précision, candidat non promu. |
| [30 oct. : tranche prospective 02](../../docs/validation/nlp-prospective-pipeline-2026-10-30.md) | 705 consensus LLM. Production : 22,66 % / 85,14 % ; candidat : 16,12 % / 93,24 %. Le candidat ajoute 6 vrais positifs et 144 faux positifs. | Test consommé ; aucun modèle ne passe les deux critères gelés. |
| [5 nov. : campagne darkweb ciblée](experiments/results/2026-11-05-darkweb-live.json) | 140 consensus sur 200, un seul positif. Production : 1 TP et 24 FP ; candidat : 0 TP et 4 FP. | Sous-puissant, non qualifié, aucune promotion. |
| [24 nov. : ingestion → extraction → graphe](../../docs/validation/2026-11-24-pipeline-smoke.md) | Passé sur deux documents, poids de fixture fixes, Ollama simulé ; provenance et rejeu sans doublon. | Régression logicielle uniquement ; aucune réserve consommée, aucun réentraînement. |
| [24 nov. : ajout du contexte CTI à l’export](../../docs/validation/2026-11-24-review-cti.md#vérifier) | Tests unitaires, catalogue KEV public et export comparé sur MongoDB jetable avec un compte en lecture seule. Seul le contexte attendu est ajouté ; le fichier enrichi reste privé et non écrasable. | Régression logicielle passée, aucun corpus de qualification consommé. |
| [Prochaine qualification humaine](../../docs/qualification-nlp-parser.md) | Aucun score : lot prospectif annoté indépendamment et arbitré à constituer ; extraction mesurée séparément. | En attente, aucune promotion. |

## Ce qui a été essayé hors qualification finale

| Sujet | Ce qu’on sait ou ce qu’il reste à vérifier | Rapports |
| --- | --- | --- |
| Labels de bootstrap, relecture humaine, lexique | Les scores dépendent de la provenance des labels. Les catégories de crime et le signal `s_identity_fraud` n’ont pas été retenus ; le cluster forums non-anglophones reste un diagnostic ciblé. | [Erreurs FP/FN](docs/cv-analyse-fp-fn-20261018.md), [politique CTI](docs/annotation-policy-v1.md), [revue groupée historique](docs/validation-revue-groupee.md) |
| Pilotes automatiques v1/v2 et premiers candidats | Des comparaisons et calibrations aveugles ont été réalisées sur des lots publics. Conserver leurs réserves consommées hors entraînement ; ne pas généraliser au trafic réel. | [Pilote](../../docs/validation/nlp-automatic-evaluation.md), [v2](../../docs/validation/nlp-automatic-v2-2026-10-19.md), [réentraînement du 19 oct.](../../docs/validation/nlp-retraining-2026-10-19.md) |
| Équilibrage et provenance | Une source prédisait la classe ; le split et le contrôle des quasi-doublons ont dû être audités. Le score du corpus équilibré n’est pas une estimation terrain. | [Corpus et candidats](../../docs/validation/nlp-balanced-training-2026-10-19.md), [audit critique](../../docs/validation/nlp-balanced-audit-2026-10-19.md), [audit NLP](../../docs/validation/nlp-audit-2026-10-20.md) |
| Itérations de modèles et features | Les rapports consignent seuils globaux/par source, têtes par source, extraction/structuration, interactions de preuve, encodeurs, groupes d’acteurs et retrait des chevauchements. Certains résultats sont exploratoires ou invalidés après audit. | [Journal itératif](../../docs/validation/nlp-iterative-audit-2026-10-20.md), [parcours complet et suites](../../docs/validation/nlp-model-parcours.md) |
| Extension de sources, transfert entre domaines, nouveaux acteurs | Les avis publics, Q&A et ransomware.live ne remplacent pas les forums du flux réel. Le rappel sur acteurs inconnus avec labels faibles est un proxy, pas un test de qualification. | [Audit des sources](../../docs/validation/nlp-source-audit-2026-10-19.md), [extension et contrôles](../../docs/validation/nlp-source-expansion-plan-2026-10-19.md), [parcours complet](../../docs/validation/nlp-model-parcours.md) |
| Annotation v3 à v11, segmentation et accord | Les petites tranches v10/v11 n’ont pas franchi la porte de stabilité. Pour v11, majorité binaire sur 20 cas ciblés mais seulement 1 unanimité ; ne pas assimiler couverture majoritaire et qualité. | [Petit pilote](../../docs/validation/nlp-small-pilot-method-2026-10-30.md), [audit d’efficacité](../../docs/validation/nlp-efficiency-audit-2026-10-31.md), [parcours v3–v9](../../docs/validation/nlp-model-parcours.md) |
| Contrat `text_cti_evidence` et labels faibles | Exiger une preuve concrète et conserver la provenance et l’incertitude ; cela ne qualifie pas un modèle. Distinguer la proposition de contrat, les audits et les campagnes réalisées. | [Proposition](../../docs/validation/nlp-label-contract-proposal.md), [protocole gelé](../../docs/validation/nlp-text-cti-corpus-protocol-v1.md) |
| Nomic vs EmbeddingGemma | Avantage d’EmbeddingGemma à rappel élevé sur trois graines. Rapports agrégés disponibles ; certains plis, caches et runs d’origine ne sont plus présents. Pas de comparaison directe EmbeddingGemma vs TF‑IDF sur ce protocole. | [Résultats sur trois graines](docs/cv-embeddinggemma-20261018.md), [limites de reproduction](docs/pistes-a-creuser-2026-11-05.md) |
| MiniLM figé vs vocabulaire TF‑IDF de production | TF‑IDF devant sur les trois graines de cette comparaison hors production ; possible avantage de vocabulaire appris sur des données chevauchantes. Ne pas généraliser à tous les encodeurs. | [Résultats agrégés](experiments/results/2026-11-06-tfidf-minilm.json), [protocole et biais](docs/pistes-a-creuser-2026-11-05.md) |
| Audit des poids, seuils et fuites OOF | Des quasi-doublons peuvent gonfler le score de développement ; une récupération de précision par source sur un lot biaisé ne qualifie pas un changement. Les hypothèses non testées, dont `min-df` en production, restent ouvertes. | [Dossier d’audit de novembre](docs/pistes-a-creuser-2026-11-05.md), [diagnostic terrain](../../docs/validation/nlp-why-no-improvement-2026-10-30.md) |

## Rejouer ou ajouter une entrée

- [Archives exécutables et tests de régression](experiments/README.md) : Go,
  analyses Python, anciennes politiques d’annotation. Les tests de code ne
  recalculent pas les campagnes ; la CI les exécute séparément.
- [Confirmation humaine](../../docs/confirmation-nlp.md) : procédure distincte
  des pilotes à labels automatiques. Les résultats synthétiques historiques
  figurent dans [`docs/validation/`](../../docs/validation/).
- Les chemins `data/...` référencés dans les rapports désignent des artefacts
  locaux ignorés par Git ; certains manquent. Conserver les originaux, les
  snapshots et les manifestes dans un stockage privé. Les JSON dans
  [`experiments/results/`](experiments/results/) ne gardent que des agrégats,
  pas le texte, les URL ou les annotations individuelles.
- Après un nouvel essai, ajouter une ligne datée et un rapport : **hypothèse ;
  version/snapshot et empreintes du corpus, des partitions et du modèle ;
  provenance des labels ; seuil et critères gelés ; couverture, métriques et
  limites ; décision ; statut de la réserve**. Si le run est bloqué ou
  invalidé, le noter au lieu de le refaire à l’identique.
