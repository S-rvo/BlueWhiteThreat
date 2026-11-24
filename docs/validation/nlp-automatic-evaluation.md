# Évaluation automatique du NLP — issue #34

Suivi : https://github.com/S-rvo/BlueWhiteThreat/issues/34

Cette campagne démarre une évaluation autonome. Elle produit des annotations
IA et conserve cette provenance ; elle ne certifie pas une vérité terrain humaine.
Aucune annotation ni mesure de ce pilote n'est utilisée pour l'entraînement ou
la promotion d'un modèle.

## Protocole figé

Le candidat est le `model.json` réellement déployé, avec le prétraitement corrigé.
`cmd/prepareautoeval` utilise les primitives de gel et de détection de nouveauté
existantes. Son fichier `automatic-protocol.json` définit le protocole automatique :
400 documents, dont 200 de validation et 200 de test ; graine 42 ; rappel cible
90 %, précision minimale 50 %, couverture d'annotation 80 %, au moins 50 positifs
et 50 négatifs consensuels dans le test. Ces objectifs sont exploratoires.
Les intervalles d'incertitude devront accompagner toute mesure.

Le manifeste structurel hérité contient un objectif de précision de 99 % propre
à `confirmfinal`. Cet objectif n'est pas celui du pilote : ne pas appeler
`confirmfinal` sur ces annotations IA. Les outils de confirmation humaine restent
inchangés et ne doivent pas servir à présenter un accord IA comme une certification.

Le gel archive 2 349 textes : les 2 325 références d'entraînement, y compris les
anciennes lignes vides pour réserver leurs IDs, et les 24 diagnostics synthétiques
précédemment inspectés. Les 14 IDs présents seulement dans les corrections
historiques sont aussi exclus ; leurs textes manquants empêchent de rechercher
d'éventuelles paraphrases. SimHash contrôle chaque candidat contre les références
et les documents déjà sélectionnés ; cette méthode lexicale ne garantit pas
l'indépendance des événements racontés.

## Premier pool et résultat de préparation

Le collecteur borné `apps/scraper/cmd/collecteval` a effectué huit requêtes à
l'API publique Stack Exchange, sans compte ni contournement. Il applique le filtre
partagé `pkg/csamfilter` avant persistance et passe les corps HTML par
`sites.ExtractMainContent`, comme le scraper. Le filtre reste lexical et ne
constitue pas une garantie sémantique complète. Il n'écrit ni dans Redis ni dans
MongoDB et ne collecte ni réponses, commentaires ni images.

797 questions ont été conservées : security 200, superuser 199, askubuntu 198,
french 200. Trois questions ont été exclues pour filtre de sûreté ou taille.
Les 487 exclusions de nouveauté laissent **310 textes sélectionnés sur 400
requis**. La sélection est scellée ; la préparation retourne une erreur explicite
et aucun couple validation/test n'est annoncé comme complet. Ne pas effacer la
sélection pour tirer de nouveau. Les annotations de ce lot sont uniquement un
diagnostic de l'outillage et de sa couverture, pas un test de qualification.

Ce pool de questions publiques n'est pas représentatif des forums Tor ni des
annonces d'accès initial. La source française concerne la langue française,
pas les menaces cyber en français. Un corpus ciblé supplémentaire sera nécessaire
dans une campagne distincte, avec un nouveau plan fixé avant ses résultats.

## Annotations locales en aveugle

`scripts/annotate_autoeval.py` utilise deux modèles Ollama disponibles localement :
`llama3.2:3b` et `gemma4:e2b`. Le plan conserve leurs empreintes réelles, celles du
script, du prompt, de la politique et de la sélection. Le modèle NLP n'est pas
appelé et aucun score n'est transmis aux annotateurs. L'accord de deux petits
modèles peut comporter des erreurs communes.

Chaque décision exige une déclaration de lecture complète, une justification et
une citation dont les positions Unicode correspondent au texte immuable. Cela
contrôle la structure des réponses, pas la véracité d'une déclaration de lecture.
Les modèles reçoivent le texte comme données non fiables et doivent ignorer ses
instructions. Un désaccord, une ambiguïté, une réponse invalide ou une limite de
contexte produit une abstention ; jamais un négatif automatique. Les documents
longs ne sont pas tronqués pour obtenir artificiellement une décision. Une panne
de transport arrête le travail, conserve les votes terminés et reste une panne.

Les annotations sont enregistrées après chaque réponse et la reprise vérifie le
plan figé. Un verrou empêche deux annotateurs de fichier concurrents. L'option
`--limit 10` traite les dix premiers textes sélectionnés pour éprouver le contrat ;
`--limit 0` poursuit toute la sélection. Une reprise ne rejuge pas les votes
précédents pour améliorer le consensus. Les textes, citations et justifications
restent dans le répertoire local ignoré par Git.

Depuis `apps/classifier` :

```sh
go run ./cmd/prepareautoeval
# Collecter depuis apps/scraper : go run ./cmd/collecteval
# La sélection de ce pilote a déjà été faite : ne pas la relancer.
python3 -B scripts/annotate_autoeval.py --limit 10
python3 -B scripts/annotate_autoeval.py --limit 0
```

Les artefacts locaux sont dans `data/automatic-eval-2026-10-19/` : pool, résumé de
collecte, modèle/corpus/code figés, sélection, plan d'annotation, votes et consensus.
Le consensus affiche séparément décisions, désaccords, abstentions, documents en
attente et couverture ; son verdict reste `inconclusive` sur ce pilote incomplet.
Aucune métrique NLP n'est inventée si les annotations ne sont pas disponibles.

## Suite avant toute conclusion sur le modèle

1. Mesurer la validité et la couverture des annotations locales ; conserver
   explicitement les limites de ces petits modèles.
2. Préparer une collecte autorisée de vrais forums, comprenant des discussions
   CTI françaises et anglaises et des négatifs difficiles ; documenter la population
   et les exclusions. Réserver un lot indépendant sans regarder les scores.
3. Geler validation et test séparés par fils et copies d'événements, conserver
   les abstentions et mesurer le taux d'accord et la couverture par strate.
4. Développer l'évaluation avec matrice de confusion, précision/rappel/F1,
   intervalles à 95 %, taux de transmission et résultats par langue/source.
5. Choisir le seuil sur validation uniquement, puis consommer une seule fois le
   test réservé. Si le compromis ou les effectifs sont insuffisants, conclure
   échec/inconclusif ; ne pas régler sur le test ni intégrer ses textes au train.
6. Évaluer un candidat alternatif si nécessaire, puis exécuter en observation
   sur une collecte suivante. Aucune promotion automatique n'est réalisée ici.

## Premier essai exécuté

Dix documents ont été annotés par les deux modèles locaux (20 votes). Résultat :
2 consensus, 2 désaccords, 6 abstentions liées à des réponses invalides. La
couverture du lot traité est donc de 20 % ; 300 documents restent sans annotation.
Ce petit essai révèle une limite de l'annotation et ne mesure pas la performance
du classifieur. Il ne justifie pas de lancer aveuglément le reste du lot avec ces
mêmes garanties insuffisantes. Aucun seuil, poids ou modèle de production n'a
été modifié dans cette campagne.

La suite prioritaire est de fiabiliser les références automatiques et de préparer
un corpus de forums ciblé. Toute évolution des prompts/modèles/contrats devra
faire l'objet d'un nouveau plan ; les votes de cet essai ne seront pas remplacés
pour augmenter artificiellement l'accord. Le résumé agrégé publiable est
`nlp-automatic-pilot-summary-2026-10-19.json`.

Vérifications : suites Go du classifieur et `go vet ./...` réussies ; tests et
`go vet` du collecteur réussis ; trois tests Python du contrat d'annotation réussis ;
`git diff --check` réussi. Les tests techniques ne prouvent pas la justesse des IA.

## Campagne suivante

Une [campagne v2 distincte](nlp-automatic-v2-2026-10-19.md) corrige le contrat des
citations, ajoute un modèle local plus capable et un nouveau pool ciblé. Ses
400 documents sont séparés en validation et test ; les votes du pilote ci-dessus
restent inchangés. Son rapport porte sa propre provenance et ses limites.
