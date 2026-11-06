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
