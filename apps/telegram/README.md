# Collecte Telegram sans compte

Service Go autonome : import des listes deepdarkCTI, découverte de canaux publics,
suivi incrémental paginé et stockage du texte. Aucune connexion à un compte,
aucune adhésion, aucun téléchargement de média. L’intégration NLP/IA reste séparée.

## Démarrer Tor

Depuis la racine du dépôt :

```sh
docker compose -f docker-compose.telegram.yaml up -d tor1
docker compose -f docker-compose.telegram.yaml logs --tail=30 tor1
```

Attendre `Bootstrapped 100%`. Le port SOCKS5 publié est `9051`.
Si un proxy écoute déjà sur ce port, le réutiliser au lieu d’en démarrer un second.
Toutes les commandes Go suivantes se lancent depuis `apps/telegram`.

## Trois modes de travail

```sh
# Import des deux listes, sans visiter les canaux :
go run ./cmd/telegram

# Exploration : vérifier au maximum 30 candidats non suivis :
go run ./cmd/telegram -mode discover -max-channels 30 -request-timeout 90s

# Suivi : au maximum 5 canaux déjà lisibles et 10 pages par canal :
go run ./cmd/telegram -mode follow -max-channels 5 -max-pages 10 -request-timeout 90s
```

`-probe` reste un alias de `-mode discover`. Les sources sont
`telegram_infostealer.md` et `telegram_threat_actors.md`. `-seed-dir /chemin`
permet de lire ces deux fichiers localement. Le suivi ne relit pas GitHub.

La découverte ne consomme pas le budget du suivi. Un canal confirmé
`web_readable` reçoit `tracked=true` : c’est une admission **technique**, pas un
verdict de pertinence CTI. Les anciens registres sont migrés automatiquement
à partir de cet état, sans avancer leurs curseurs. Un canal temporairement en
erreur reste dans la file de suivi. Les invitations restent non visitées.
Les liens des messages et des transferts alimentent les candidats du cycle
suivant, avec l’URL du message d’origine comme provenance.

Par défaut, le collecteur explore jusqu’à **deux liens** depuis une source
des listes importées ou ajoutée directement au registre, et inscrit au plus
**50 nouvelles destinations par cycle** (invitations incluses, jamais visitées).
Les options sont `-max-discovery-depth 2` et `-max-new-candidates 50`.
Une profondeur de zéro limite les requêtes aux sources de départ.
Un budget de zéro empêche l’ajout de nouvelles destinations par les messages.
Un candidat déjà connu ne consomme pas le budget une seconde fois.

La profondeur est le plus court chemin calculé depuis les provenances
existantes. Les candidats sans chemin autorisé restent dans le registre avec
leurs curseurs, mais sont écartés des deux files. Le bilan affiche
`new_candidates`, `outside_depth`, `links_skipped_depth` et
`links_skipped_budget`. Les liens écartés ne sont pas mis en attente ;
ils restent présents dans les messages stockés en JSON ou dans MongoDB.


## Stockage durable, sans NLP

Le suivi écrit par défaut un fichier JSON par message dans :

```text
data/outbox/<canal>/<message_id>.json
```

Une relance remplace atomiquement le fichier du même message. Chaque document
contient URL, canal, identifiant du message, dates, texte et liens Telegram.
Cela constitue une boîte de sortie locale utilisable par une future analyse.
`-outbox-dir /chemin` change son emplacement. La découverte seule, sans
`-store`, reste un échantillonnage en mémoire et ne conserve pas les textes.
En mode `all`, une boîte de sortie est disponible pour les deux files.

Pour écrire directement dans MongoDB, configurer `MONGO_URI` dans
l’environnement et éventuellement `MONGO_DB` (défaut `BlueWhiteThreat`) :

```sh
go run ./cmd/telegram -mode follow -store -max-channels 5 -max-pages 10
```

La collection est `article`, avec les champs `url`, `source`, `scraped_at`,
`content`, `channel`, `message_id`, `published_at`, `telegram_links` et un `_id`
déterministe. Aucun classifieur, fournisseur de modèle ou service NLP n’est lancé.
Une connexion MongoDB locale ne passe pas par le transport HTTP Tor.

Un curseur est associé à sa destination de stockage. Changer de boîte de sortie
ou de connexion MongoDB avec le même registre est refusé pour ne pas sauter les
messages déjà parcourus. Utiliser un registre distinct (`-registry`) et refaire
l’import/découverte pour alimenter une autre destination.

## Pagination et reprise

Le registre `data/candidates.json` conserve :

| Champ | Rôle |
|---|---|
| `last_message_id` | Dernière borne entièrement stockée |
| `catchup_head` | Message le plus récent du rattrapage en cours |
| `before_id` | Prochaine page plus ancienne à demander |
| `followed_at` | Dernière tentative de suivi |
| `next_check_at` | Prochaine tentative autorisée |
| `failures` | Erreurs successives servant au délai de reprise |

Le suivi commence par la page récente, puis demande `?before=<id>` à partir du
contrôle de pagination de l’aperçu. Tous les textes d’une page sont stockés
avant de sauvegarder son curseur. La borne `last_message_id` n’avance que lorsque
l’intervalle jusqu’à l’ancienne borne a été parcouru, ou que l’historique exposé
par l’aperçu est épuisé. Les publications ne contenant que des médias comptent
pour la pagination mais ne sont pas téléchargées.

Si `-max-pages` est atteint, `complete=false` est normal : le cycle suivant
reprend à `before_id`. Les messages arrivés pendant ce rattrapage seront lus
après sa fin. Une panne de stockage rejoue la page sans sauter de message ; les
écritures sont idempotentes. Telegram peut fournir à la fois un lien vers les
messages anciens (`before`) et un lien vers les messages récents (`after`).
Le suivi n’utilise que `before`. Une pagination qui ne recule pas ou des dates
de message invalides reportent le canal sans avancer son curseur ; le cycle
continue sur les autres canaux.

Le registre est sauvegardé après chaque page stockée et chaque vérification,
ainsi qu’en sortie gérée. Les écritures utilisent un fichier temporaire, `fsync`
et un remplacement atomique. Un verrou système `.lock` empêche deux processus
d’écrire dans le même registre. Le système libère le verrou après un crash ;
ne pas supprimer le fichier de verrou pendant qu’un processus tourne.

## Exécution périodique

```sh
# Les deux files dans un seul processus, sans chevauchement des cycles :
go run ./cmd/telegram -mode all -interval 5m -follow-interval 10m \
  -discovery-interval 24h -retry-interval 15m \
  -max-channels 5 -max-pages 10 -request-timeout 90s
```

`-interval 0` exécute un seul cycle. Sinon, le processus attend cet intervalle
après chaque cycle et recommence. `Ctrl+C` interrompt le travail et sauvegarde
la progression. Une erreur réseau n’entraîne jamais de connexion directe ; le
planificateur réessaie après son attente. Les échecs par source sont différés
avec un délai exponentiel plafonné à 24 heures. Les sources indisponibles sont
réévaluées après `-discovery-interval` ; les suivis réussis après
`-follow-interval`. Les sources les moins récemment traitées passent en premier.
Le budget `-max-channels` s’applique à chacune des deux files en mode `all`.
Les canaux nouvellement découverts entrent dans le suivi à partir d’un cycle
ultérieur. Un rafraîchissement GitHub en erreur n’empêche pas les files existantes
de travailler en mode `all` ; aucun demi-import n’est enregistré.

Le planificateur garde le verrou pendant ses attentes. Pour utiliser le même
registre, choisir un processus `all` plutôt que deux processus concurrents.

Depuis la racine, l’équivalent Docker (Tor et collecte, sortie JSON dans le volume) :

```sh
docker compose -f docker-compose.telegram.yaml up -d --build telegram
docker compose -f docker-compose.telegram.yaml logs -f telegram
```

Docker utilise `socks5h://tor1:9050`, `-mode all -interval 5m`, et le volume
`telegram_registry` pour le registre et la boîte de sortie. La configuration
est autonome et ne requiert pas les `.env` des autres services.

## Accès, réseau et limites

| État | Signification |
|---|---|
| `pending` | Candidat non vérifié |
| `invitation_required` | Invitation non visitée |
| `web_readable` | Publications reconnues dans l’aperçu public |
| `preview_unavailable` | Pas d’aperçu, ou redirection vers la fiche du canal |
| `check_failed` | Erreur réseau, HTTP, format ou pagination |
| `blocked_content` | Page rejetée par le filtre partagé |

Les requêtes GitHub et Telegram passent obligatoirement par SOCKS5, avec
résolution distante des destinations et sans cookies ni suivi de redirections.
Le proxy par défaut est `socks5h://127.0.0.1:9051`, configurable par
`TOR_PROXY_URL` ou `-tor-proxy`. `HTTP_PROXY`, `HTTPS_PROXY` et `NO_PROXY` sont
ignorés. Un proxy SOCKS5 arbitraire ne prouve pas qu’il s’agit de Tor.

Chaque réponse est limitée à 4 MiB et chaque requête à 60 secondes par défaut
(`-request-timeout`). Les requêtes Telegram sont espacées de deux secondes
(`-delay`). Le filtre lexical partagé `pkg/csamfilter` est appliqué avant
stockage ou découverte. Il ne fournit pas de détection multimodale.

La complétude concerne les publications encore exposées par l’aperçu public.
Les messages supprimés, historiques restreints, changements de nom des canaux
et contenus réservés à l’application ne sont pas récupérables par ce parcours.
La détection des modifications/suppressions d’anciens messages n’est pas incluse.
L’admission CTI par NLP/IA reste à intégrer séparément.

## Tests

```sh
go test ./...
go vet ./...
```

Tests sans canal tiers : séparation des files, reprise d’un rattrapage borné,
messages arrivant pendant le rattrapage, stockage défaillant, pagination bloquée,
sortie JSON, verrou interprocessus, arrêt du planificateur, filtrage et transport
SOCKS5 sans repli direct. Le test MongoDB/classifieur existant est facultatif et
ne fait pas partie du planificateur ; voir
`apps/classifier/cmd/main/telegram_integration_test.go`.
