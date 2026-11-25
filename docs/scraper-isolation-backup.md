# Isolation du scraper et sauvegarde Redis

## Scraper

Compose utilise une image de production, sans montage du dépôt, en UID/GID
`10001:10001`, sans capabilities et avec `no-new-privileges`. La racine est en
lecture seule ; Chromium utilise des espaces temporaires bornés pour son
profil (/tmp) et sa mémoire partagée (/dev/shm). La configuration de sites
par défaut est vide. Si une configuration est nécessaire, monter ce seul
fichier en lecture seule et indiquer son chemin via `SITES_CONFIG_PATH` ;
ne pas remonter le dépôt. Pour les connecteurs qui utilisent un fichier
d’état, monter un répertoire privé dédié accessible à l’UID 10001.
Les contextes des deux Dockerfiles du scraper n’incluent que le code Go.
Les secrets injectés par Compose restent hors de l’image.
Toute ancienne image construite avec le Dockerfile de développement
non filtré doit être considérée comme contenant les secrets locaux ;
ne pas la distribuer et renouveler les secrets si elle a été partagée.

Le profil `ops/scraper/seccomp.json` est dérivé du profil Moby
`seccomp/v0.2.1` (SHA-256 du profil d’origine :
`536529b665dd0972c37bfb569f5d4ac8a53592e7b00752bc39ff063ca9864c74`).
Il ajoute seulement les variantes de `clone` d’espaces de noms requises par
Chromium, `unshare(CLONE_NEWUSER)` et `chroot` (les droits restent
contrôlés par le noyau). Ni `SYS_ADMIN` ni `seccomp=unconfined` ne sont
nécessaires. L’hôte Linux doit accepter les espaces de noms utilisateur non
privilégiés. Le démarrage s’arrête si Chromium ne confirme pas le sandbox
d’espaces de noms PID et réseau, et le sandbox seccomp-BPF. Le contrôle seul
se lance avec :

```sh
docker compose run --rm --no-deps scraper --check-browser-sandbox
```

## Redis

Le conteneur Redis lui-même n’est pas redémarré par ce changement. Le service
`redis-backup` exporte dans un fichier temporaire, contrôle le RDB avec
`redis-check-rdb`, puis publie le fichier par renommage atomique. Il refait
un export toutes les 24 h après succès et réessaie après 15 min en cas
d’échec, notamment si Redis charge encore ses données. Il garde les
14 dernières sauvegardes automatiques et ne supprime pas les fichiers
nommés manuellement hors de ce schéma. Le statut Docker vérifie la présence
d’une sauvegarde non vide de moins de 26 h. Il ne remplace ni un essai de
restauration sur un Redis séparé, ni une copie hors du disque du Mac.
Ce service ne sauvegarde pas MongoDB.

```sh
docker compose ps redis-backup
docker compose logs --tail 30 redis-backup
```
