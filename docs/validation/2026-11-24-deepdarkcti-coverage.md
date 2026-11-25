# Couverture DeepDarkCTI — 24 novembre 2026

[DeepDarkCTI](https://github.com/fastfire/deepdarkCTI) est le périmètre de ce
rapport. C'est un catalogue de références CTI, pas un inventaire complet des
services Tor ni une liste de sites qui acceptent la collecte automatisée.
Le rapport reconnaît uniquement la provenance enregistrée pour ce catalogue.
Il ne change pas les sources configurées dans le collecteur, sa file, ou ses
règles d'accès. Il ne télécharge pas le catalogue et ne visite aucune adresse.

En complément, le [suivi des sources CTI publiques](../cti-public-sources.md)
vérifie la fraîcheur de DeepDarkCTI et des flux officiels. Ce suivi mesure la
disponibilité des sources, pas leur couverture dans Redis ou MongoDB.

## Rejouer

Depuis la racine, avec Python 3.10+ et les services Redis et MongoDB du Compose
local démarrés :

```sh
python3 apps/crawler/scripts/coverage_report.py
python3 -B -m unittest discover -s apps/crawler/scripts
RUN_COVERAGE_MONGO_TEST=1 python3 -B -m unittest discover -s apps/crawler/scripts
```

Le test Mongo utilise des documents synthétiques fournis en mémoire dans
l'agrégation ; il ne crée aucune collection et n'insère aucune donnée.
Le rapport n'exécute que des lectures. Son client Mongo réutilise toutefois
l'identité du conteneur local ; pour un déploiement ailleurs, prévoir un compte
de lecture dédié. Le JSON n'exporte ni adresse, ni contenu, ni identifiant de
connexion.

## Lire le JSON

| Indicateur | Ce qu'il mesure |
| --- | --- |
| `attributed_seeds`, `seeds` | Points de départ dont Redis conserve une provenance DeepDarkCTI reconnue, au total et par fichier du catalogue. Le collecteur ne renseigne pas tous les doublons et les métadonnées expirent : ce n'est pas le nombre de liens du catalogue. |
| `attempted_seeds`, `successful_seeds` | Points de départ avec une trace de tentative ou de réussite ; le succès exige une réponse 2xx/304 enregistrée par le crawler. Ce n'est pas le nombre de pages extraites par le scraper. |
| `hosts_with_http_401_or_403` | Hôtes attribués à un seul fichier avec au moins une réponse 401 ou 403 enregistrée dans les compteurs encore présents. Une réponse 403 ne prouve pas la présence d'un formulaire de connexion et un zéro ne prouve pas que le site est public. |
| `ambiguous_hosts` | Hôtes associés à plusieurs fichiers dans les données observées. Ils sont exclus des compteurs par hôte et MongoDB pour éviter les doubles comptes ; leurs points de départ restent comptés. |
| `documents`, `useful_verdicts` | Documents MongoDB sous le même hôte (et port non standard) qu'un point de départ attribué ; parmi eux, ceux dont le verdict enregistré est `relevant=true` et `needs_review=false`. Ce rapprochement ne prouve pas la provenance de chaque page. |

Le rapport lit jusqu'à 100 000 clés de page distinctes et impose un délai de
60 secondes. La lecture Redis n'est pas un instantané : même complète, elle
porte uniquement sur les clés encore observables. Lorsque `scan_complete=false`,
les résultats sont partiels, y compris la détection des hôtes ambigus. Si
`attributed_seeds=0`, le rapport n'interroge pas MongoDB : il manque la
provenance nécessaire pour en interpréter les résultats, ce qui ne dit rien du
contenu déjà stocké.

Un verdict utile enregistré n'est pas une mise à jour ni une décision de
rétention. Pour estimer l'effet du modèle et de la politique actuels, utiliser
la [simulation de conservation](2026-11-24-retention-shadow.md) et garder la
suppression désactivée tant que les rejets n'ont pas été audités.

## Vérification locale

Le 24 novembre 2026 à 18 h 54 (Paris) : 24 759 clés de page examinées, scan
complet, aucune provenance DeepDarkCTI reconnue. Il n'y a donc pas encore de
mesure de couverture ou de rendement par fichier sur cette instance.
Les tests de comptage, d'ambiguïté, de limite du scan et l'agrégation MongoDB
sur documents synthétiques ont réussi.
