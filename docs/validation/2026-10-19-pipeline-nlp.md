# Validation du pipeline jusqu'au NLP — 19 octobre 2026

## Mise à jour après répétitions et correction de compatibilité

30 passages sur chacun de 24 textes synthétiques : 720 inférences, scores et décisions identiques à chaque passage. Après correction, les 12 positifs synthétiques restent sous le seuil et les 12 négatifs sont rejetés. Ce lot diagnostique ne mesure pas la qualité générale du modèle. Le texte CTI initial passe de 0,067861 à 0,5265 environ, toujours sous le seuil de 0,581961.

Une incompatibilité de vectorisation a été corrigée : le modèle historique déclare `char_ngrams=true` et utilise les fragments `@c:` de longueur 3–5, bornés par `^` et `$`, ainsi que des fréquences brutes. Le code récent utilisait des fragments `c_` de longueur 4 et `log1p`. Le chargement respecte désormais le format historique lorsque son champ est présent, tout en conservant le comportement récent pour les nouveaux modèles. Les poids et le seuil restent inchangés.

Vérification sur 50 lignes retrouvées parmi les erreurs du rapport historique, chacune classée 30 fois dans le conteneur : 1 500 inférences stables, scores exactement égaux au rapport (écart maximal nul). Ce sous-ensemble sélectionné pour ses erreurs ne permet aucune estimation de F1 générale. Le F1 historique est 0,7707 ; il ne signifie pas qu’un même texte passe aléatoirement huit fois sur dix.

Résultats reproductibles : `nlp-synthetic-fixtures-2026-10-19.json`, `nlp-repeat-results-before-fix-2026-10-19.json`, `nlp-repeat-results-2026-10-19.json` et `nlp-historical-repeats-2026-10-19.json`. Le programme `apps/classifier/experiments/cmd/repeatprobe` recharge le modèle à chaque répétition. Exemple depuis le dossier classifier : `go run ./experiments/cmd/repeatprobe --model model.json --fixtures ../../docs/validation/nlp-synthetic-fixtures-2026-10-19.json --repeat 30`.

La pipeline réelle extraction → MongoDB → NLP a également été rejouée après reconstruction : tests réussis, score positif 0,526527, score négatif 0,056291, reclassification du texte modifié vérifiée. Les tests Go et `go vet` du classifieur passent.

Les sections suivantes conservent les observations du test initial, avant cette correction ; leurs anciens scores et leur analyse des huit caractéristiques ne décrivent plus l’inférence corrigée.

## Verdict du test initial

Après correction et nouvel essai, les tests de fonctionnement passent : extraction navigateur → MongoDB → classifieur Docker → verdict en base, puis reclassification d'un texte modifié avec le même modèle. L'ancien verdict est retiré atomiquement lors d'une modification et une inférence sur un texte obsolète ne peut plus être écrite.

Le faux négatif sémantique reste présent : l'exemple CTI synthétique obtient toujours un score de 0,067861. Le test de transport ne demande plus une justesse sémantique de 100 % ; il vérifie la sortie, sa version, sa cohérence avec le seuil et l'exclusion déterministe des annuaires, et signale les erreurs de qualité via `MODEL_QUALITY_MISS`. Ce changement du test ne constitue pas une amélioration du modèle. Aucun poids, vocabulaire ou seuil n'a été modifié.

## Méthode et périmètre

Test opt-in dans `apps/scraper/internal/sites/pipeline_integration_test.go` avec HTML synthétique servi par un serveur HTTP local dans le conteneur scraper. Appels des fonctions réelles `scrapeOneGuarded`, `GetUnscrapedURLs`, `MarkURLScraped`, `SaveScrapedPage` ; MongoDB réel ; résultat écrit par le binaire de `classifier-prd` réellement en service. Le classifieur a été reconstruit avec la protection contre les écritures obsolètes, puis redémarré après création des fixtures et après modification du texte pour déclencher ses cycles immédiatement, sans modifier son intervalle de 10 minutes.

Une instance Redis éphémère sur le port 6380, sans chargement de dump existant, a isolé les sets de test des sets actifs du crawler. Les fixtures MongoDB ont été supprimées par les nettoyages des tests ; l'instance Redis de test a été arrêtée sans sauvegarde. Le conteneur NLP reste actif.

Ce test vérifie les fonctions de passage Redis/scraper sur des fixtures ; il n'exécute pas la boucle complète `RunDarkwebScraper` sur la file active. Les URL localhost sont accessibles directement par Chromium : le transport Tor, la découverte de liens par le crawler, les comptes de forums et les cadences réelles de 30/10 minutes ne sont donc pas validés par cet essai. La séparation exploration/suivi reste une cible documentaire.

## Résultats

| Contrôle | Résultat |
|---|---|
| Lecture Redis des pages non traitées et marquage | Réussi sur l'instance isolée |
| Extraction HTML via Chromium | Réussie ; navigation et pied de page exclus |
| Texte, URL source et date en MongoDB | Vérifiés sur trois pages |
| Deux scrapes de la même URL | Un seul document par URL dans cet essai séquentiel |
| Marqueur synthétique du filtre de sûreté | Rejet avant persistance ; aucun contenu interdit réel utilisé |
| Sortie NLP persistée | Trois verdicts avec score, modèle et politique |
| Exemple CTI positif | Échec : `relevant=false`, score `0.067861` |
| Exemple cuisine/jardinage négatif | Réussi : `relevant=false`, score `0.036249` |
| Annuaire de liens | Réussi : exclusion par la règle annuaire, score `0` |
| Mise à jour du texte | Corrigé : ancien verdict retiré atomiquement |
| Contenu identique | Verdict conservé malgré une nouvelle date de scraping |
| Nouveau classement après édition | Vérifié avec le même modèle : score passé de 0,067861 à 0,036249, conforme au nouveau texte |
| Inférence concurrente sur un ancien texte | Rejetée ; inférence sur le texte actuel acceptée, test MongoDB réel réussi |
| Texte commençant par `$` | Conservé littéralement par la mise à jour MongoDB |
| Tests existants du classifieur et `go vet ./...` | Réussis |
| Tests existants du scraper et `go vet ./...` | Réussis ; tests Redis/Mongo historiques ignorés dans cette exécution pour protéger les données actives, remplacés par l'essai isolé ci-dessus |

Les tests d'intégration opt-in passent après correction. Les suites normales les ignorent sauf si `BWT_PIPELINE_TEST=1`. Les fixtures sont nettoyées après exécution. Le test de transport exige aussi un Redis de test vide sur un port différent de 6379 ; il refuse une instance non vide avant toute mutation. Le test de concurrence est dans `apps/classifier/internal/db/mongo_integration_test.go`.

## Diagnostic et suites nécessaires

1. Le modèle monté et le fichier hôte ont la même empreinte SHA-256 : `0c00b06c8f48df847d3ef813ac45d8f1689d49736912746c4862b9c29dbdbb89`. Version `2026-10-17T20-52-35`, seuil déclaré `0.5819606191352668`. Le rejet CTI n'est donc pas expliqué par un ancien fichier dans un volume de synchronisation. Il faut analyser les faux négatifs sur un corpus indépendant ; ce seul exemple ne suffit pas à mesurer la qualité générale.
2. Le rapport d'évaluation fourni avec ce modèle indique 401 exemples de test, précision 91,86 %, rappel 66,39 %, F1 77,07 % ; `target_95_reached=false`. Ce sont des résultats du rapport existant, pas une nouvelle évaluation indépendante.
3. **Corrigé** : `SaveScrapedPage` utilise une mise à jour MongoDB en pipeline, conditionnelle sur le texte antérieur, pour retirer `relevance` uniquement si le texte change. `UpdateRelevance` exige que le texte en base corresponde exactement au texte classé avant de publier le verdict. Un résultat obsolète est écarté ; le document sans verdict reste éligible au prochain cycle.
4. Chromium 155 émet des erreurs de décodage CDP `unknown IPAddressSpace value: Loopback` avec les dépendances navigateur actuelles. L'extraction locale aboutit malgré ces erreurs ; la compatibilité des versions doit être corrigée et retestée.
5. La boucle de scraping marque actuellement une URL comme traitée même après un échec. Le mécanisme de reprises bornées décrit dans le cadrage n'est donc pas encore assuré par cette boucle ; constat de lecture du code, pas un scénario réseau validé ici.

## Analyse du faux négatif

L'inférence directe avec le fichier hôte reproduit exactement le score du conteneur : `0.06786066507487984`. Sur ce texte synthétique, le vocabulaire du modèle ne reconnaît que huit caractéristiques : `and`, `security`, `the`, `of`, `example`, `address`, `data`, `malware`. `ransomware`, `phishing` et `cve` sont absents de son vocabulaire. Le biais vaut `-3.319644`. Cela explique le score faible sans démontrer un défaut de transport ou un fichier monté incorrect.

L'amélioration de ce faux négatif nécessite un travail de couverture du corpus/vocabulaire et une nouvelle évaluation séparée de l'entraînement. Modifier le seuil sur cet exemple seul ou ajouter le texte de test à l'entraînement ne démontrerait pas une amélioration générale. Le taux d'erreur ne peut pas être déduit de ces trois fixtures.
