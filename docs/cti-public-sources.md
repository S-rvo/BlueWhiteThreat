# Suivi des sources CTI publiques

| Source | Indicateur de fraîcheur |
| --- | --- |
| [DeepDarkCTI](https://github.com/fastfire/deepdarkCTI) | Dernier push signalé par GitHub ; ce n'est pas la date de mise à jour de chaque fichier ou site. |
| [CERT-FR — Menaces et incidents](https://cert.cyber.gouv.fr/cti/) | Date la plus récente des publications du RSS. |
| [CERT-FR — Alertes de sécurité](https://cert.cyber.gouv.fr/alerte/) | Date la plus récente des alertes du RSS. |
| [CERT-EU — Threat intelligence](https://cert.europa.eu/publications/threat-intelligence/) | Dernière publication disponible dans le RSS des bulletins. |
| [CISA — Known Exploited Vulnerabilities](https://github.com/cisagov/kev-data) | Date de publication et nombre d'entrées du catalogue JSON via le miroir officiel. |
| [SANS Internet Storm Center](https://isc.sans.edu/diary.html) | Date la plus récente indiquée dans le RSS du journal. |

Le suivi ne consulte que les flux et le point d'accès aux métadonnées du
dépôt. Il ne suit ni les liens des publications ni les redirections, n'ajoute
aucune URL au crawler et ne conserve aucun texte. Le nombre d'entrées RSS
correspond à la fenêtre publiée, pas à l'intégralité des archives. Un flux
en état `ok` est accessible et lisible ; sa cadence éditoriale peut varier.

Depuis la racine :

```sh
(cd apps/scraper && go run ./cmd/collectcti -feed-status)
```

Le [workflow GitHub Actions](../.github/workflows/cti-feed-status.yml) publie
ce rapport dans le résumé de chaque exécution. Il est prévu à 00 h 27 et
12 h 27 UTC ainsi qu'au déclenchement manuel, une fois le workflow présent sur
la branche par défaut et Actions activé. Si une source échoue, le JSON conserve
les autres résultats et la commande retourne un code non nul.

**Vérification locale du 24 novembre 2026 :** les six sources ont répondu.
La couverture des données déjà stockées reste dans le
[rapport DeepDarkCTI](validation/2026-11-24-deepdarkcti-coverage.md).

[L'export privé de revue](validation/2026-11-24-review-cti.md) peut aussi
rapprocher les CVE mentionnées du catalogue KEV et afficher les liens CERT-FR
pour la vérification manuelle. Les autres flux restent des références.
