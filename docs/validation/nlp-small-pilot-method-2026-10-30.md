# Petit pilote avant toute montée en charge — 2026-10-30

## But
Décider si le protocole d'annotation et le correctif de caractéristiques valent la peine d'être étendus, sans lancer des milliers d'annotations. Aucun changement de modèle production, de seuil ou de corpus de test consommé.

## Pilote gelé
Le seul lot disponible est le pool post-test `development-pool-v1.jsonl` (91 documents, 91 hôtes; export du 30 octobre). Les annotations v9 déjà lancées restent limitées à ce lot. Le manifeste `development-pool-v1-pilot-split.json` fixe avant lecture des labels une partition déterministe, sans labels : deux tiers pour le développement et un tiers en réserve. Les hôtes sont disjoints par construction dans ce pool.

- Développement : entraîner au plus un candidat expérimental, en plus des seules données d'entraînement antérieures déjà admissibles. Ne pas toucher `model.json` de production.
- Réserve : ne pas lire ses labels, ne pas l'utiliser pour entraîner, choisir les caractéristiques ou régler un seuil avant gel d'un candidat unique. Avec environ 30 exemples, elle est uniquement diagnostique et ne constitue pas une preuve de promotion.
- Annotation : trois modèles IA locaux figés, consensus majoritaire. Cela mesure la stabilité entre annotateurs IA, pas la vérité des labels.

## Portes de décision
1. **Qualité d'annotation** — calculer couverture, abstentions, désaccords, taux d'accord par paire et motifs de désaccord sur la partie développement. Si beaucoup de désaccords/abstentions ou si le même cas de politique revient, corriger le guide et refaire un petit lot seulement; ne pas scaler.
2. **Utilité du correctif** — comparer l'ancien modèle et un seul candidat sur développement groupé, avec erreurs par catégorie (prix sans preuve, pages de casino/boutique, faux négatifs explicites). N'ajouter des labels que si la limite identifiée nécessite réellement plus d'exemples. Aucun réglage sur la réserve.
3. **Diagnostic aveugle** — après gel du candidat et du seuil, ouvrir la réserve une seule fois. La taille est trop petite pour promotion; elle sert seulement à repérer une régression flagrante ou une annotation incohérente.
4. **Décision de scaler** — uniquement si l'annotation est assez stable, que le gain est net sur développement et que la réserve ne révèle pas de régression manifeste. Alors seulement collecter une nouvelle tranche prospective plafonnée (p. ex. 100–200), l'annoter, puis répéter les portes. Chaque tranche doit justifier la suivante; aucun lot de 4 000 à l'avance.
5. **Promotion** — nécessite un nouveau test prospectif assez grand, indépendant, au taux de base réel, avec critères fixés avant ouverture. Les tests consommés précédents restent intacts et exclus de tout entraînement/réglage.

## Limite statistique
91 pages ne suffisent pas à certifier les performances de production, surtout avec une prévalence positive faible. Sans adjudication humaine, trois modèles IA ne fournissent qu'une mesure de cohérence, jamais une garantie de vérité terrain. Le pilote tranche la valeur de continuer à investir, pas l'aptitude au déploiement.

## État opérationnel
L'annotation v9 du lot de 91 est terminée et ses reçus sont conservés. Pas d'autre collecte ni annotation massive ne sera lancée dans ce pilote. Aucune promotion, aucun commit ni push.

## Résultat observé du premier lot (partie développement seulement)

Le run v9 a terminé : 91/91 documents passés par les trois modèles locaux; 88 consensus, 2 abstentions et 1 désaccord (couverture consensus 96,7 %). Sur les 61 IDs de développement gelés, les 61 ont un consensus majoritaire, avec 52 négatifs et 9 positifs. L'accord pair à pair sur les décisions disponibles est de 71,7 % à 81,7 % selon la paire. Ce résultat indique une bonne couverture du mécanisme de consensus, mais une stabilité individuelle inégale et un très petit nombre de positifs. Ce ne sont pas des labels vérifiés humainement.

**Décision : ne pas scaler et ne pas entraîner/promouvoir de nouveau candidat sur la base de ce lot.** Neuf positifs ne permettent pas de comparer sérieusement la récupération des cas positifs; une réserve de 30 pages serait également trop petite pour estimer les performances. La réserve de 30 IDs reste masquée à toute analyse par document. Les tests prospectifs déjà consommés restent exclus.

Pour la prochaine mini-tranche, préenregistrer les critères avant annotation : couverture consensus >=90 %, accord pair à pair >=80 % pour chaque paire, et au moins 30 positifs de développement pour un test directionnel (pas de promotion). Si le premier critère échoue, revoir le guide; si les deux premiers passent mais qu'il manque des positifs, n'annoter qu'une tranche ciblée et plafonnée d'exemples contrastifs, sans toucher à la réserve. Pour toute promotion, garder les portes déjà fixées : nouveau test prospectif représentatif, >=50 exemples de chaque classe et précision >=50 % avec rappel >=90 %.

## Petit test du guide v10 (développement uniquement)

L'audit des 61 exemples de développement a surtout isolé deux ambiguïtés : le mot « incident » pouvait être compris comme un événement physique/non cyber; les offres d'identifiants/clefs volés ou de services cyber malveillants n'étaient pas toujours distinguées des annonces commerciales génériques. Le script v9 et son plan gelé restent inchangés. Une nouvelle v10 précise ces frontières; 12 documents en désaccord, sélectionnés uniquement dans les 61 IDs de développement, ont été annotés par les mêmes trois modèles (36 votes supplémentaires).

Sur cette tranche volontairement difficile, l'accord pair moyen est passé de 33,3 % (v9) à 44,4 % (v10); les 12 ont un consensus majoritaire dans les deux versions et une seule décision majoritaire change. C'est un progrès directionnel faible, non représentatif et non vérifié par humain; cela ne passe pas la porte de montée en charge. **Arrêt ici : pas d'autre annotation, pas d'entraînement, pas de seuil ni promotion.** Détails agrégés et empreintes : `development-pool-v1-v10-disagreement-slice-audit.json`. La réserve de 30 IDs reste non consultée.
