# Plan d'extension des sources NLP — 19 octobre 2026

## Objectif

Constituer un corpus de 2 000 à 4 000 textes réellement diversifiés, sans
transformer la provenance en étiquette. Chaque source alimente une réserve non
scorée ; les classes viennent ensuite de l'annotation automatique consensuelle.
Les doublons exacts, proches, les mêmes CVE et les mêmes événements restent dans
un seul split.

## Sources prioritaires

| Source | Signal utile | Rôle attendu | Volume visé | Accès / provenance |
|---|---|---|---:|---|
| ransomware.live, plusieurs mois | revendications, profils de victimes, annonces de fuite | positifs et négatifs difficiles de même style | 1 500 | API publique v2 |
| RansomLook | publications et groupes ransomware | réserve positive proche de la cible | 500 | API/export public, à auditer avant collecte |
| CERT-FR alertes, CTI et avis | français officiel, incidents et vulnérabilités | positifs officiels + négatifs cyber proches | 500 | API/RSS officiels |
| CERT-EU Threat Intelligence | rapports d'incidents en anglais | positifs officiels | 250 | RSS officiel |
| CISA KEV | vulnérabilités exploitées, avec/sans ransomware | contraste ransomware/CVE | 500 | JSON officiel |
| GitHub Advisory Database | descriptions de vulnérabilités | négatifs cyber difficiles | 1 200 | API publique GitHub |
| MITRE ATT&CK STIX 2.1 | groupes, logiciels, campagnes, techniques | vocabulaire CTI et positifs officiels | 750 | dépôt officiel, licence MITRE |
| MISP/CIRCL OSINT | événements et rapports STIX/MISP | réserve CTI multi-source | 500 | feeds publics déclarés par MISP |
| abuse.ch ThreatFox | descriptions d'IOC et familles | contexte menace concis | 300 | API/feed public |
| abuse.ch MalwareBazaar | métadonnées et signatures malware | négatifs/positifs cyber courts | 300 | API/feed public |
| Stack Exchange Security | questions/réponses conversationnelles | styles conversationnels positifs et négatifs | 800 | export/API, attribution CC BY-SA |
| dépôts GitHub de rapports CTI sous licence explicite | rapports, IOC, analyses de campagnes | positifs variés | 500 | dépôt et licence enregistrés par document |
| Wikipédia/Wikidata, sociétés et produits | profils sans incident | négatifs généraux correspondant aux bios de victimes | 750 | API/export avec attribution |
| bulletins éditeurs (Microsoft, Cisco Talos, Unit 42, Sophos, Securelist) | analyses de campagnes | positifs éditoriaux | 500 | RSS/pages publiques ; conditions vérifiées source par source |

## Ce qui est déjà collecté

- ransomware.live octobre : 432 documents ;
- ransomware.live septembre : 610 documents, dont un test de 200 désormais
  consommé et donc exclu de tout entraînement futur ;
- ransomware.live août : 699 documents, dont un contrôle indépendant consommé
  de 200 documents et 169 consensus automatiques ;
- GitHub Advisories : 1 200 documents supplémentaires collectés dans
  `apps/classifier/data/source-expansion-2026-10-19/ghsa-1200.jsonl`, sans label
  automatique.

## Équilibre cible

La sélection finale doit plafonner chaque source, groupe et événement. Cible
minimale : 500 exemples dans chacune des quatre cellules suivantes : positif
conversationnel, positif officiel, négatif conversationnel difficile et
négatif officiel/commercial. Français et anglais sont suivis séparément. Les
petites classes linguistiques restent visibles dans le rapport au lieu d'être
absorbées dans une moyenne globale.

## Expériences de modèle

1. **TF-IDF mots + caractères.** Le code courant combine déjà unigrammes,
   bigrammes de mots et 4-grammes de caractères. La prochaine comparaison fera
   varier les réserves de vocabulaire et la fréquence documentaire uniquement
   en validation croisée de développement.
2. **Encodeur compact figé.** Premier candidat :
   `sentence-transformers/paraphrase-multilingual-MiniLM-L12-v2`, Apache-2.0,
   50 langues et vecteurs de dimension 384.
3. **Petite tête seulement.** L'encodeur reste figé ; seule une régression
   logistique est entraînée sur les embeddings. Cela limite le surapprentissage
   et le coût de production.
4. **Comparaison embeddings + logistique.** Même splits groupés, même métriques
   et même gel du test que le TF-IDF.
5. **LLM local.** Qwen et Gemma restent des annotateurs indépendants. Le modèle
   de production demeure un classifieur déterministe et mesurable.

## Règles de qualification

Une source n'est jamais une classe. Aucun résultat déjà lu ne sert à choisir un
ratio, un seuil ou une architecture. Toute nouvelle famille de modèles est figée
avant un nouveau test, lequel exige au moins 20 positifs et 100 négatifs
consensuels répartis entre plusieurs sources et groupes. Les abstentions et les
désaccords ne sont pas convertis artificiellement en labels.

## Références principales

- CERT-FR API : https://www.cert.ssi.gouv.fr/openapi/
- CERT-FR RSS : https://www.cert.ssi.gouv.fr/flux-rss/
- GitHub Global Advisories API : https://docs.github.com/en/rest/security-advisories/global-advisories
- MITRE ATT&CK STIX : https://github.com/mitre-attack/attack-stix-data
- MISP default feeds : https://www.misp-project.org/feeds/
- MiniLM multilingue : https://huggingface.co/sentence-transformers/paraphrase-multilingual-MiniLM-L12-v2
- Sentence Transformers : https://www.sbert.net/docs/sentence_transformer/training_overview.html

## Résultat du contrôle d’août

Les deux artefacts et leurs seuils ont été figés avant scoring. Sur 169 consensus
automatiques (28 positifs, 141 négatifs), le TF-IDF atteint 73,0 % de précision,
96,4 % de rappel et 83,1 % de F1. La tête MiniLM atteint 85,7 %, 64,3 % et
73,5 %. Le premier résultat est encourageant mais reste mono-source et compte
moins de 50 positifs : le lot est consommé sans promotion ni réglage ultérieur.
Les détails reproductibles sont dans
`apps/classifier/data/source-expansion-2026-10-19/ransomware-live-2026-08-result.json`.
