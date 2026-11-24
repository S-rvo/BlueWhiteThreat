# Contexte CVE pour la revue du classifier

Depuis `apps/classifier`, avec la configuration Mongo habituelle de l'export :

```sh
go run ./cmd/export -cti-context
```

L'option fournit aux personnes chargées de la revue un champ `cti_context`
sur les documents déjà marqués `needs_review` et contenant des identifiants
CVE ou CERT-FR. Le fichier par défaut est `review_queue.cti.jsonl` (ignoré
par Git). Il contient le texte collecté : garder le fichier local et privé.
L'export enrichi crée le fichier avec des droits `0600` et n'écrase
pas un fichier existant. `EXPORT_OUTPUT` permet de choisir un autre chemin ;
pour un chemin personnalisé, vérifier qu'il ne sera pas ajouté à Git.

- Pour chaque CVE repérée dans le texte, `listed_in_kev` indique si elle
  figure dans cette version du [catalogue officiel CISA KEV](https://github.com/cisagov/kev-data).
  `kev_released_at` date la version. Une absence signifie seulement
  « non listée dans cet instantané », pas « jamais exploitée ».
- Une référence explicite `CERTFR-…-ALE-…` ou `CERTFR-…-AVI-…` devient
  un lien vers l'alerte ou l'avis correspondant, à vérifier manuellement.
  Sinon, utiliser [les avis du CERT-FR](https://cert.cyber.gouv.fr/avis/) et
  le numéro de CVE pour la recherche manuelle. L'export n'annonce pas qu'un
  avis CERT-FR existe pour chaque CVE.

Le rapprochement est fait localement et n'envoie ni document, ni URL source,
ni CVE extraite aux éditeurs. Un seul GET borné lit l'instantané public KEV ;
les liens CERT-FR ne sont pas visités. Pour rejouer avec un instantané local
et sans requête KEV :

```sh
go run ./cmd/export -cti-context -kev-file /chemin/known_exploited_vulnerabilities.json
```

Le contexte ne modifie ni l'ordre, ni les scores, ni les labels ; une
correspondance ne valide pas le contenu d'une page. Aucun réglage ou
réentraînement du modèle n'est effectué.

## Vérifier

```sh
go test -race ./cmd/export
BWT_TEST_PUBLIC_KEV=1 go test -v ./cmd/export -run '^TestKEVPublisherSnapshot$'
```

Les tests utilisent des documents synthétiques pour couvrir la déduplication,
les correspondances présentes et absentes, les références CERT-FR et le rejet
d'un catalogue invalide. Ils vérifient aussi la lecture d'un instantané local,
l'échec sans appel réseau si celui-ci manque et le cas d'une référence CERT-FR
sans CVE. Le test réseau optionnel vérifie la lecture du flux public, sans
connexion à MongoDB.

Depuis la racine, `python3 -B tests/pipeline/smoke.py` exécute aussi
`TestSmokeCTIReviewExport`. Il lance le vrai binaire avec un compte en lecture
seule sur une base MongoDB jetable. Sur deux documents synthétiques, l'export
enrichi et l'export standard gardent les mêmes champs d'origine, les mêmes
scores et le même ordre. Seul le document marqué à revoir reçoit du contexte.
Le test contrôle aussi les droits `0600` et le refus d'écraser le fichier.

**Résultat local du 24 novembre 2026 :** tests Go avec `-race`, `go vet`,
lecture du catalogue officiel et smoke test complet passés. Le premier essai
d'intégration avait produit un export vide : la fixture n'atteignait pas le
minimum de trois mots-clés du filtre de sélection existant. Seule la fixture
a été corrigée avant le rejeu, pas le comportement du classifieur.
