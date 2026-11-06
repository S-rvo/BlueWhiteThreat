# Connecteurs : inscription, session et challenges

Dernière mise à jour : 6 novembre 2026.

Évolution optionnelle du scraper, désactivée en l'absence de configuration. Aucun site réel n'est préconfiguré. Cette extension dépasse le périmètre MVP décrit dans `mvp-2027-cadrage.md` (§ Collecte des forums) ; les exclusions du MVP restent son contrat historique, et ne décrivent pas cette extension.

## Préparer une nouvelle installation

Sur cet environnement, la clé a déjà été générée et validée : conserver le fichier existant. Sur une nouvelle machine uniquement, depuis la racine du dépôt, créer la clé sans l’afficher et sans écraser une clé existante :

```bash
python3 - <<'PYKEY'
import base64
import os
from pathlib import Path

path = Path("apps/scraper/.credentials.env")
fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
with os.fdopen(fd, "w") as file:
    file.write("BWT_CREDENTIALS_KEY=" + base64.b64encode(os.urandom(32)).decode() + "\n")
print("Clé créée ; valeur masquée.")
PYKEY

git check-ignore apps/scraper/.credentials.env
```

Si le fichier existe déjà, la création s’arrête sans le modifier. Le principal exécutable Go charge ce fichier depuis son répertoire de travail `apps/scraper`. Les commandes auxiliaires ne bénéficient pas automatiquement de ce chargement. Docker Compose utilise le chemin déclaré dans `env_file`.

**Attention au build Docker :** être ignoré par Git ne suffit pas à exclure un secret du contexte de build. Le Dockerfile de développement utilise `COPY apps/scraper/. .`. Avant de construire ou distribuer une image, exclure `.credentials.env`, les autres fichiers de secrets et les états de session via le `.dockerignore` du contexte concerné ; charger les secrets à l’exécution seulement. La validation Compose ne vérifie pas cette isolation.

## Configuration

Partir de `apps/scraper/examples/sites.json`, adapter les sélecteurs aux formulaires et définir `SITES_CONFIG_PATH`. Les variables `FORUM_USER`, `FORUM_PASSWORD`, `FORUM_EMAIL` contiennent un compte stable, pas des identités régénérées. Avec `state_file`, les identifiants et champs d’inscription sont persistés chiffrés ; `credentials_key_env` vaut par défaut `BWT_CREDENTIALS_KEY` si omis. Sans fichier d’état, les identifiants restent uniquement dans l’environnement. Les cookies restent des secrets persistés en fichier 0600. Monter un répertoire privé persistant pour `state_file`, distinct par domaine. Ne pas versionner ces fichiers.

Le parcours est : restauration des cookies → contrôle du sélecteur de connexion → inscription optionnelle une fois → connexion → confirmation → collecte → sauvegarde des cookies. Les fichiers d'état sont remplacés atomiquement. Les opérations d'un même fichier sont sérialisées dans le processus. Utiliser un seul processus par fichier d'état ; le verrou n'est pas distribué.

Le marqueur `registration_attempted` est écrit avant la navigation d'inscription. Une erreur ou un délai dépassé ne provoque donc pas une nouvelle création au cycle suivant. En cas d'échec ambigu, vérifier l'état du compte avant toute remise à zéro manuelle du marqueur. Pour un compte existant, omettre `registration`.

## Résolution de challenge

Le connecteur transmet `{"challenge_html":"…"}` par POST JSON au `solver_url` configuré et attend `{"answer":"…"}`. Il remplit `answer_selector` une fois. Un `submit_selector` optionnel permet de valider un challenge autonome ; le connecteur attend alors la disparition du challenge. Sinon, la réponse est soumise avec le formulaire de connexion/inscription.

HTTPS est exigé hors loopback ; le jeton de `token_env` est transmis en Bearer. Délai du solveur : 20 secondes, borné aussi par le contexte du fetch ; redirections désactivées, tailles bornées. Aucun fournisseur commercial n'est intégré. L'adaptateur externe est à réaliser selon le type de challenge. Le HTML du composant peut contenir des données sensibles et son envoi doit être pris en compte dans le choix du service. Les images distantes ne sont pas téléchargées par cet adaptateur.

Sans solveur ou sans réponse, le résultat est `human_action_required`. Pas de gestion générique de reCAPTCHA, Turnstile, MFA, invitations ou validation e-mail. Pas de détection universelle : seuls les sélecteurs configurés sont reconnus. Les sélecteurs de succès doivent représenter un état authentifié réel, pas un élément présent sur toutes les pages.

## Limites opérationnelles

Le délai global existant reste applicable. La politique actuelle du worker marque aussi les URL en échec comme traitées : une reprise après intervention nécessite donc leur remise en file par l'opérateur. Pas de retry automatique ajouté. Les sessions reposent sur les cookies, pas sur localStorage/IndexedDB. Les contrôles de contenu et le pipeline d'analyse existants restent inchangés.

Tests : `cd apps/scraper && go test ./...`. Les tests unitaires couvrent état, validation et contrat HTTP du solveur ; une validation navigateur sur les formulaires cibles reste nécessaire avant activation.

## Comptes liés au site et réutilisation

Avec `state_file`, la conservation chiffrée utilise par défaut `BWT_CREDENTIALS_KEY`. Définir explicitement `credentials_key_env` seulement pour choisir une autre variable. Préparer une clé une seule fois comme décrit ci-dessus, puis la conserver dans un gestionnaire de secrets ou l’environnement privé du service. Conserver la même clé entre les redémarrages, séparément du fichier d’état et hors du dépôt. Sa perte rend les identifiants enregistrés illisibles. Sa modification seule ne réalise pas de rotation des données existantes.

Le chiffrement est AES-256-GCM avec nonce aléatoire et origine du site authentifiée. Le fichier conserve `origin`, les identifiants chiffrés, `registration_attempted`, `last_login_at` (connexion confirmée) et les cookies. L’origine comprend schéma, hôte et port : un même fichier ne peut pas être réutilisé pour un autre site. Les fichiers sans origine issus de la première version sont acceptés ; utiliser exclusivement un fichier par site dès leur création.

Au premier passage, les valeurs sont lues dans les variables configurées et enregistrées avant toute soumission d’inscription. Aux passages suivants, le compte enregistré est prioritaire, même si les variables initiales ont changé ou disparu. Les noms des variables restent les clés de configuration : les garder stables. Les champs d’inscription (e-mail, confirmation, etc.) suivent le même chiffrement. Le jeton du solveur reste uniquement dans son environnement.

Les cookies sont essayés en premier ; si la session a expiré, le compte enregistré sert à se reconnecter. Une inscription déjà tentée reste bloquée contre les répétitions. Pour remplacer un compte, arrêter le worker puis archiver son fichier d’état dans un emplacement privé et reconfigurer le compte avant redémarrage.

**Pertinence :** ce registre indique qu’un accès est enregistré et réutilisable, pas que la source est pertinente CTI. Les verdicts NLP/IA et l’admission au suivi restent indépendants ; aucun identifiant n’est ajouté au contenu analysé.

## Configuration locale installée

La clé locale persistante est dans `apps/scraper/.credentials.env` (permissions 0600, ignoré par Git). Docker Compose transmet ce fichier uniquement au service scraper. Le point d’entrée principal Go le charge aussi lors d’un lancement depuis `apps/scraper` ; les variables déjà exportées restent prioritaires. Ne pas afficher cette clé ni la régénérer lors d’un redémarrage. Sauvegarder ce fichier séparément des états de comptes.

Les sites ayant `state_file` utilisent automatiquement `credentials_key_env: BWT_CREDENTIALS_KEY` sauf clé personnalisée explicite. `data/sites.json` est encore vide : aucun compte ni formulaire réel n’a été configuré.

Après configuration des sites, recréer le service pour charger le nouvel environnement : `docker compose up -d --no-deps --force-recreate scraper`. Cette opération n’a pas été exécutée pendant la configuration.

Vérification : `go test ./...`, `go test -race ./internal/sites` et `go vet ./...` depuis `apps/scraper`. Tests de chiffrement/restauration, origine, mauvaise clé, champs d’inscription, état et permissions, contrat HTTP du solveur et clé par défaut. Ces tests ne remplacent pas la validation navigateur des formulaires cibles.

## Champs de configuration

| Champ | Usage |
|---|---|
| `login_url` | URL de connexion, même origine que la page cible |
| `username_selector`, `password_selector`, `submit_selector` | Sélecteurs CSS de connexion |
| `success_selector` | Élément confirmant réellement l’authentification |
| `username_env`, `password_env` | Noms stables des variables du compte initial |
| `state_file` | Fichier privé persistant, un par site |
| `credentials_key_env` | Variable de clé, défaut `BWT_CREDENTIALS_KEY` si `state_file` existe |
| `registration` | URL, champs CSS → variables, soumission et confirmation ; optionnel |
| `captcha` | Sélecteur du challenge, champ réponse, solveur, jeton et soumission optionnelle |

Le chemin `/run/bwt-sessions/example.json` de l’exemple exige un montage persistant pour survivre à une recréation du conteneur. Avec le montage actuel du dépôt sur `/app`, un chemin tel que `/app/db/scraper-sessions/example.json` peut être utilisé dans le conteneur ; `/db` à la racine du dépôt est ignoré par Git. Pour un lancement natif, adapter ce chemin au système local. Les répertoires existants conservent leurs permissions : vérifier leur confidentialité, pas seulement celle du fichier.

## Diagnostic et reprise

| Symptôme | Vérification / action |
|---|---|
| `credential key must be base64 of 32 bytes` | Vérifier présence et format de la variable choisie sans afficher sa valeur |
| `credential decryption failed` | Restaurer la clé d’origine ; ne pas créer une nouvelle clé pour relire l’ancien compte |
| `state_file belongs to another site` | Corriger le chemin ou le domaine ; ne pas modifier manuellement l’origine du ciphertext |
| `credentials belong to another site` | Vérifier l’association compte/origine ; le chiffrement authentifie cette association |
| `human_action_required` | Vérifier le challenge configuré, son adaptateur ou réaliser l’intervention attendue |
| Inscription sans confirmation | Vérifier le compte côté site avant de remettre un marqueur à zéro |
| Compte inchangé après modification des variables | Comportement prévu : les identifiants persistés sont prioritaires |
| URL abandonnée après erreur | Vérifier `urls_scraped` et effectuer une remise en file ciblée ; pas de purge globale |

Ne pas inclure de mot de passe, cookie, jeton ni clé dans les logs de diagnostic, tickets, corpus ou captures. L’archivage d’un état conserve des secrets : le protéger et définir sa rétention.
