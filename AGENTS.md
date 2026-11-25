# Protocole git — BlueWhiteThreat

Ce repo est travaillé en parallèle par plusieurs agents (Claude Code, Codex)
et par l'utilisateur. Ce fichier est la source unique du protocole ; il est
lu automatiquement par Codex (`AGENTS.md`) et par Claude Code (`CLAUDE.md`
l'importe). Ne pas dupliquer son contenu ailleurs.

## Convention de branche

Trunk-based : commits directs sur `main`, pas de `develop`/`feature/*`
obligatoire. Ce n'est pas une absence de process, c'est la convention
historique de ce repo (vérifiable via `git log --all --oneline --graph`).

## Avant tout commit

1. `git fetch origin` — obligatoire, imposé techniquement par
   `.githooks/pre-commit` (rejette le commit si le dernier fetch date de
   plus de 10 minutes). Objectif : ne jamais committer à l'aveugle sans
   savoir si un autre agent a déjà avancé entre-temps.
2. `git status` avant tout `add` large — ne jamais `git add -A`/`git add .`.
   Lister les fichiers explicitement, pour ne pas embarquer le travail en
   cours d'un autre agent dans son propre commit.
3. Ne committer que les fichiers liés à la tâche en cours. Un pile de
   fichiers non liés déjà dirty dans l'arbre de travail (souvent un autre
   agent en cours) ne doit jamais être ramassé par accident.

## Message de commit

Format imposé techniquement par `.githooks/commit-msg` :
`type(scope): description`, avec `type` dans
`feat|fix|docs|chore|tune|security|refactor|test|perf`. Rejeté sinon.

## Jamais sans demande explicite de l'utilisateur

- Commit (sauf autorisation donnée pour ce tour précis, non reconductible).
- Push vers un remote.
- `--no-verify`, `--amend` sur un commit déjà poussé, `reset --hard`,
  `push --force`, suppression de branche.
- Retrain/promotion d'un modèle classifier (`apps/classifier/model.json*`) —
  règle "Document now, retrain decision later".

## Activation des hooks (une fois par clone/worktree)

```
git config core.hooksPath .githooks
```

## Graphify à la demande — Codex et Claude Code

- Utiliser Graphify pour explorer une architecture, suivre un flux entre
  modules ou repérer les dépendances avant un refactoring. Pour une petite
  modification déjà localisée, lire le code directement.
- Choisir le graphe le plus ciblé : `apps/classifier/graphify-out/graph.json`
  pour le classifier, `graphify-out/graph.json` pour une question transverse.
  Lancer `graphify query "<symbole ou question>" --graph <chemin> --budget 1000`.
- Si le graphe utile manque ou que le code concerné a changé depuis sa
  génération, lancer `graphify update <périmètre>` depuis la racine du dépôt
  (`apps/classifier`, un autre service, ou `.`), puis relancer la requête.
  Pas de génération systématique au démarrage ni après chaque modification.
- Le graphe oriente la recherche ; vérifier les relations importantes dans
  les fichiers sources. Si Graphify est absent ou ne donne rien d'utile,
  continuer avec `rg`, la lecture du code et les tests.

## Reprise des essais du classifier

Avant de proposer ou lancer un nouvel essai, consulter
[`apps/classifier/EXPERIMENTS.md`](apps/classifier/EXPERIMENTS.md). Après une
campagne, y référencer le verdict et le rapport, même si l’essai échoue ou
est invalidé. Ne jamais remettre une réserve consommée dans le développement.
