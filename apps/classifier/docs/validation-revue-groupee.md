# Préparer des corrections humaines cohérentes

`scripts/validate_grouped_review.py` valide une copie remplie du formulaire groupé avant de préparer des corrections. Il ne modifie ni le modèle, ni `reviewed_labels.csv`.

Copier `data/persistent-annotations-assistant-20261015-v2/grouped-review.csv`, puis renseigner uniquement `human_label`, `human_reason` et `human_reviewer` pour les documents réellement revus. Laisser les trois champs vides pour les documents en attente. Un label doit être 0 ou 1 ; un motif et un nom de relecteur sont obligatoires. Une abstention reste en attente, sans inventer un label négatif.

```sh
python3 scripts/validate_grouped_review.py \
  --completed /chemin/revue-remplie.csv \
  --out /chemin/rapport-validation.json \
  --export /chemin/decisions-humaines.jsonl
```

L'export est facultatif et refuse un lot vide. Le rapport et l'export refusent d'écraser un fichier existant. Le JSONL est compatible avec l'importeur `cmd/applyreview`, mais aucun import n'est déclenché automatiquement. Le rapport conserve les empreintes des entrées et les motifs des décisions.

Le validateur contrôle les identifiants d'entraînement autorisés, la composition des groupes, les labels humains d'origine, les champs figés du formulaire et l'empreinte du corpus de référence. Un groupe touché doit conserver un label cohérent après application virtuelle des décisions ; aucune correction n'est propagée automatiquement à ses autres membres. Les groupes non revus restent en attente. Si deux quasi-doublons nécessitent légitimement des labels différents, il faut revoir leur regroupement plutôt que forcer une correction avec cet outil.

Les propositions de l'assistant ne sont jamais exportées à la place des champs humains. Les noms explicites `assistant`, `codex`, `claude` et `ai` sont refusés comme relecteurs humains. Ces champs restent déclaratifs : le programme n'authentifie pas l'identité du relecteur. L'original du formulaire, l'audit et le manifeste doivent provenir du dossier de revue de confiance.

La vérification du 15 octobre conserve 67 documents en attente et zéro décision humaine (`data/grouped-review-readiness-with-corpus-20261015.json`). Il s'agit d'une amélioration du circuit de correction, sans gain de précision mesuré. Les annotations humaines éventuelles serviront à un nouvel entraînement de développement ; elles ne transformeront pas ces documents déjà consultés en test indépendant.
