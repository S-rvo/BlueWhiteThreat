# Audit d'efficacité pour améliorer le classifieur — 31 octobre 2026

## Conclusion courte

Le goulot n'est pas le nombre de paramètres ni le volume brut d'annotations. Le candidat actuel classe moins bien les pages de la vraie pipeline que la production (AUROC 0,821 vs 0,838; average precision 0,426 vs 0,471). Le seuil plus bas augmente le rappel mais fait exploser les faux positifs. Il faut corriger la séparation des signaux et la représentativité des données avant de scaler.

À prévalence observée de 10,5 %, atteindre précision 50 % avec rappel 90 % demande un taux de faux positifs d'environ 10,6 % au maximum. Le modèle production est à 34,1 %; le candidat à 56,9 %. Sur 631 négatifs, cela correspond à réduire les faux positifs de 215 à environ 67 tout en conservant près de 67 vrais positifs. Changer uniquement le seuil ne suffit pas : l'AUROC/AP du candidat sont déjà inférieurs.

## Diagnostic priorisé par rendement attendu

1. **Réduire les faux positifs commerciaux.** 162/215 faux positifs production contiennent un prix; 37/39 pages négatives contenant « casino » et 34/35 contenant « betting » sont signalées positives. Les poids prix/commercial sont positifs. Le correctif d'interaction prix/preuve est uniquement une caractéristique de code : les modèles figés l'ignorent et aucun effet qualité n'a encore été mesuré. Le prochain entraînement devrait apprendre explicitement des paires contrastives : prix/commerce sans preuve vs rançon ou données volées avec preuve; annonce générique vs service cyber malveillant concret.
2. **Séparer apprentissage et mesure.** Les pages ciblées/équilibrées peuvent enrichir les exemples difficiles d'entraînement, mais ne servent pas à estimer la précision de production. Garder un flux prospectif tiré sans score au taux de base réel pour calibration et test final. Groupes hôtes, acteurs et quasi-doublons doivent rester dans une seule partition.
3. **Améliorer le texte avant d'élargir le modèle.** Le scraper applique déjà `ExtractMainContent`, qui supprime certains tags/éléments de navigation. Donc « mauvais extracteur » n'est pas un fait établi. Mesurer sur le lot de développement les longueurs, répétitions inter-pages, parts de liens/menus et résidus de template; construire un test A/B de contenu extrait vs version actuelle, sans labels de test. Les faux positifs longs (médiane 3 931 caractères) rendent ce contrôle prioritaire.
4. **Garder un modèle simple et comparer les représentations, pas la taille.** Le classifieur est une régression logistique TF-IDF avec unigrams, bigrams, n-grammes caractères et indicateurs structuraux. C'est un baseline adapté. Tester seulement 2–3 variantes préenregistrées (actuel; filtrage principal; interactions preuve/contexte) sur mêmes folds groupés et même population. Décision sur average precision, FPR à rappel 90 %, métriques par source, et pas F1 seul.
5. **Réduire le coût d'annotation sans prétendre créer une vérité terrain.** Le pool de 91 donne 88 consensus, 2 abstentions et 1 désaccord; sur 61 exemples de développement, les accords pair à pair sont 71,7–81,7 %. Le test v10 de 12 désaccords améliore l'accord moyen de 33,3 à 44,4 % sur ce sous-ensemble ciblé, mais ne change qu'une décision majoritaire. Ne pas réannoter en masse. Pour un prochain pilote, comparer les deux premiers annotateurs, solliciter le troisième sur désaccords et auditer aléatoirement une fraction des accords, seulement après gel du protocole. Cette économie doit être validée; l'accord IA n'est pas une vérité indépendante.

## Protocole minimal recommandé

### Étape 0 — sans annotation
- Profiler extraction et déduplication sur les documents déjà gelés du pool de développement.
- Mesurer les catégories fréquentes de faux positifs avec motifs structuraux, sans regarder les labels de la réserve.
- Définir explicitement le contrat positif/négatif/abstention et geler v10. Aucun réentraînement à ce stade.

### Étape 1 — mini-tranche ciblée
- Collecter au maximum 40–60 pages inédites postérieures, choisies pour couvrir des contrastes difficiles, pas pour simuler la prévalence.
- Construire des paires proches (même type/source ou template) avec un seul élément qui change : simple prix vs rançon explicite; boutique vs données volées; produit de sécurité générique vs exploit concret; bug logiciel vs vulnérabilité exploitable; incident physique vs cyberincident.
- Annoter avec les règles séquentielles seulement si leur qualité est vérifiée sur un mini-échantillon; enregistrer les évidences et désaccords. Pas de modèle NLP dans les prompts.
- Entraîner au plus un candidat isolé; vérifier qu'il réduit les erreurs ciblées sur validation groupée. Si aucun gain net, arrêter et diagnostiquer extraction/contrat, pas ajouter des milliers de pages.

### Étape 2 — go/no-go avant un test réel
Scaler seulement si le candidat améliore l'average precision et réduit le FPR à rappel comparable sur des groupes non vus, sans détérioration importante par source. Sinon, ne pas lancer la grande annotation prospective.

### Étape 3 — qualification finale
Après gel d'un seul candidat et d'un seul seuil, collecter un test prospectif aléatoire indépendant au taux de base de la pipeline. Le test consommé le 30 octobre reste exclu. Environ 700 pages avaient donné 74 positifs dans le précédent échantillon; une future tranche d'ordre similaire est nécessaire pour obtenir assez de positifs au taux réel, mais elle ne doit être annotée qu'après franchissement des étapes 0–2. Promotion uniquement si précision >=50 %, rappel >=90 %, et effectifs/classes préenregistrés sont satisfaits.

## Limites et décision actuelle

Les tests disponibles utilisent un consensus LLM, pas une référence humaine. Les trois annotateurs ont une dépendance possible de politique et d'erreurs; aucune méthode exclusivement IA ne peut garantir la justesse réelle. Le test ciblé v10 porte sur 12 cas difficiles, pas sur un échantillon représentatif. Le pool ne contient que 9 positifs parmi les 61 exemples de développement et ne permet pas d'estimer robustement le rappel.

**Décision au 31 octobre : ne pas annoter plusieurs milliers de pages et ne pas entraîner/promouvoir de candidat.** Les 30 IDs de réserve restent fermés. Priorité immédiate : mesurer sans labels la qualité d'extraction/déduplication sur le pool existant, puis n'engager qu'une mini-tranche ciblée si un signal testable en ressort.

## Étape zéro exécutée : profil sans labels du développement

Profil calculé uniquement sur les 61 IDs de développement; aucun label consulté et aucune donnée des 30 IDs de réserve analysée. Le texte est déjà le contenu extrait par la pipeline, pas le HTML source. Résultats : longueur médiane 2 339 caractères, p90 10 484, minimum 5; 8/61 pages font moins de 200 caractères et 12/61 moins de 500. Aucun doublon exact normalisé, une paire dépasse 0,8 de Jaccard sur shingles de 5 mots; 6 pages seulement partagent des lignes longues avec au moins trois pages. 29/61 contiennent un motif de prix. Cela confirme quelques pages d'extraction potentiellement pauvres et beaucoup de signaux de prix, mais ne prouve pas que le boilerplate soit la cause dominante sur ce pool.

Le modèle de production, appliqué sans labels aux 61 documents, prédit 30 positifs; parmi les 8 textes de moins de 200 caractères, un seul est positif. Ne pas convertir cette petite tranche en estimation de performance. Ajouter un contrôle qualité distinct de la classification : pour les textes très courts, tenter une nouvelle extraction/collecte ou différer la décision; ne pas forcer automatiquement la classe négative. Le seuil de longueur doit être évalué sur développement, pas choisi à partir des tests consommés.

L'extracteur en place est `apps/scraper/internal/sites/ExtractMainContent`, appelé par `FetchPage`; les tests ciblés `go test ./internal/sites ./tests` passent. L'étape suivante la plus économique est donc un contrôle/retry des pages trop courtes et un audit ciblé des segments partagés, avant de refaire l'architecture du classifieur. Résumé reproductible : `development-pool-v1-content-profile-dev-only.json`.

## Étape 1 exécutée : mini-tranche fraîche et porte de stabilité

La base Mongo a fourni 187 pages inédites après la coupure précédente; elles sont toutes postérieures au 30 octobre 12:52 UTC et viennent de 187 hôtes. Les textes déjà présents dans les corpus/test et les 91 pages du premier pilote ont été exclus par normalisation exacte. Le profil a appliqué le modèle de production gelé uniquement pour sélectionner un challenge de développement, jamais pour annoter. Le lot gelé contient 54 pages >500 caractères et dans la limite de contexte, en trois strates : 20 risques commerce/prix à score positif sans motif d'évidence lexical, 20 cas sous le seuil proches du seuil, 14 autres scores positifs. Les 54 hôtes sont nouveaux par rapport aux deux tests précédents et au pool initial. Ce lot est ciblé et n'est pas représentatif.

L'annotation v10 donne 53 consensus sur 54 (98,1 % de couverture), 39 négatifs, 14 positifs et 1 désaccord. Toutefois, les accords paire à paire sont Qwen/Gemma 73,6 %, Qwen/Llama 81,1 %, Gemma/Llama 72,2 %. Les votes positifs sont 19 pour Qwen, 18 pour Llama et 5 pour Gemma. **La porte préenregistrée (>=80 % pour chaque paire) échoue** : la couverture consensus ne suffit pas à masquer le biais important entre annotateurs. La strate commerce/prix contient 7 consensus positifs sur 20; elle ne peut donc pas être traitée comme négative par règle simple.

## Décision finale du cycle

Arrêt au gate de stabilité d'annotation. Pas de candidat entraîné, pas de comparaison modèle sur cette tranche, pas de nouvel ajustement de prompt, pas d'autre tranche d'annotation, pas de réglage de seuil ou promotion. Les résultats n'autorisent ni à scaler ni à conclure que v10 est meilleur que v9. Le lot de 54 est développement ciblé uniquement; les 30 IDs réservés du premier pilote restent fermés. Le modèle production conserve son empreinte inchangée. Les votes, plan gelé et reçus sont conservés; un rapport dérivé corrige la chaîne d'autorité v9 erronée dans le code de sortie v10 sans modifier les votes. Détails agrégés : `apps/classifier/data/iteration-2026-10-27-source-refresh/small-pilot-2026-10-31/mini-pilot-v1-annotation-audit.json`.

## Correctif de traçabilité et test v11 (31 octobre)

Pour traiter l'instabilité sans consommer une grande tranche, un mini-lot de 20 documents a été extrait des désaccords individuels observés dans le lot ciblé de 54. Il sert uniquement au développement du protocole, pas à une mesure indépendante : la sélection dépend des votes v10. La v11 exige une citation de segment pour chaque critère positif et interdit les citations pour les critères négatifs; le label est calculé par le code à partir des critères. Le schéma et la validation sont couverts par six tests unitaires. Cette validation garantit que les références sont structurées et pointent vers un segment, pas que le segment justifie sémantiquement le critère.

Résultat : le remaniement n'a pas stabilisé les trois annotateurs. Sur les mêmes 20 cas difficiles, l'accord pair-à-pair v10 était Qwen/Gemma 5/19 (26,3 %), Qwen/Llama 9/19 (47,4 %), Gemma/Llama 5/20 (25,0 %). En v11, il est 6/20 (30,0 %), 15/20 (75,0 %), et 1/20 (5,0 %). Aucun couple n'atteint le gate de 80 %. V11 donne une majorité binaire sur les 20 documents, mais 19/20 ne sont pas unanimes : cette couverture majoritaire ne constitue pas une preuve de qualité. Seule une décision majoritaire sur 20 change entre v10 et v11; les changements de vote par modèle sont Qwen 5, Gemma 1, Llama 5. L'évidence formelle n'a donc pas corrigé la divergence des interprétations.

**Correctif appliqué :** ajout de `scripts/audit_annotation_agreement.py`, qui distingue unanimité, majorité non unanime, absence de majorité et votes insuffisants, et calcule les accords pair-à-pair. Il corrige un défaut de lecture du KPI « consensus coverage » : avec trois votes binaires complets, une majorité existe presque toujours, même lorsque les modèles divergent fortement. Les rapports JSON rejouables sont `mini-pilot-v1-agreement-audit-v10.json`, `mini-pilot-v1-agreement-audit-v11.json` et `mini-pilot-v1-agreement-challenge-v10.json` dans le répertoire du mini-pilote.

**Décision : no-go annotation, pas d'entraînement, pas de seuil/model change, pas de promotion, pas de scaling.** On arrête les itérations de prompt sur cette tranche ciblée; tout gain apparent serait un résultat d'optimisation sur les cas ayant déjà révélé le désaccord. Pour établir la justesse des labels sans revue humaine, il reste nécessaire d'obtenir une référence externe indépendante/une définition de tâche vérifiable; les trois modèles locaux seuls ne donnent pas cette garantie. Les 30 IDs réservés restent non consultés.

### Sous-audit automatisé des critères v11

L'audit automatique par critère confirme que la divergence est surtout une différence de seuil/interprétation, pas un défaut de format JSON : dans ce lot ciblé de 20, Gemma ne coche aucun critère positif, Qwen coche `malicious_campaign_or_service` sur 12 et Llama `stolen_or_leaked_data` sur 17. Ces fréquences ne sont pas des taux d'erreur et ne se généralisent pas au corpus; elles démontrent toutefois que la majorité entre annotateurs ne peut pas être assimilée à une vérité stable. Le rapport JSON d'accord inclut désormais l'accord au niveau des critères et les fréquences positives par modèle. Sur ce lot, exiger l'unanimité ne conserverait qu'1 document sur 20 : trop peu pour alimenter un entraînement. Cela confirme le no-go et évite de lancer une annotation coûteuse.

### Correctif intégré au résumé v11

Le premier correctif calculait l'accord dans un rapport à part. J'ai aussi corrigé `annotation_v11.py` pour que les futures sorties `consensus.json` exposent par document `vote_agreement` (unanimous/majority_only/...) et, au résumé, ces effectifs ainsi que l'accord paire à paire. Ainsi, le champ historique `consensus_coverage_completed` ne peut plus être interprété seul comme stabilité. Le snapshot et les reçus du run v11 déjà exécuté restent inchangés; ce changement s'applique uniquement à un nouveau run et produit donc un nouveau hash de script. Test de régression ajouté : une majorité 2–1 doit s'afficher comme `majority_only`, avec les accords pair-à-paire exacts.

### Porte stricte pour les futures annotations automatiques

J'ai ajouté à la sortie v11 un champ `strict_label`, rempli uniquement si les trois modèles donnent exactement le même label; dans tout autre cas il reste `null`. Le label majoritaire historique demeure visible à titre diagnostique, mais n'est pas confondu avec cette porte plus conservatrice. Le résumé indique aussi `strict_label_count`. Les résultats existants ne sont pas recalculés : sur le lot v10 complet, 34/54 votes étaient unanimes; sur le challenge v11 ciblé, seulement 1/20. Cela montre que la règle protège contre la divergence, mais ne génère pas assez de labels pour le challenge et ne prouve toujours pas l'exactitude des labels unanimes. Test ajouté pour les ballots 2–1 et 3–0. Aucun résultat de ces lots n'est injecté à l'entraînement.

## Disponibilité d'un nouveau mini-lot — contrôle lecture seule (1er novembre)

Avant toute nouvelle annotation, un export en lecture seule de la pipeline a vérifié la disponibilité d'exemples frais après exclusion du corpus précédent et des 187 pages déjà exportées. L'export propre retourne **0 document inédit** : le lot 187 reste la dernière tranche disponible; ses exemples ne doivent pas être réutilisés comme validation indépendante. Une première tentative a produit 3 492 lignes avec un fichier d'exclusion incomplet/mal formé; ces artefacts ont été explicitement rejetés et ne sont utilisables ni pour l'annotation, ni pour l'entraînement, ni comme preuve de nouveauté. Ils sont conservés avec leurs empreintes dans `fresh-pipeline-availability-audit-2026-11-01.json` pour rendre le statut traçable. Aucune annotation n'a été lancée. La prochaine boucle attend une collecte réellement nouvelle, puis gèle l'échantillon et l'exclusion avant toute annotation.
