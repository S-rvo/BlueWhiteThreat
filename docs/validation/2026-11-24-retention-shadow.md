# Simulation de conservation — 24 novembre 2026

**Hypothèse.** En décidant page par page, on peut limiter le stockage
permanent aux textes jugés pertinents sans demande de revue. Une page
conservée peut justifier de référencer le site, pas de conserver tout son
contenu. Ce rapport ne compte ni les sites ni les octets économisés.

## Rejouer

Depuis `apps/classifier`, injecter `RETENTION_MONGODB_URI` via la configuration
privée d’un compte MongoDB en lecture seule, avec `MONGO_DB` si la base
n’est pas `BlueWhiteThreat`. Utiliser le même `MODEL_PATH` et, s’il existe,
le même `RELEVANCE_THRESHOLD` que le service actif :

```sh
go run ./cmd/main -retention-report
```

La commande effectue une agrégation bornée sur les métadonnées des verdicts
dans `article`. Elle renvoie uniquement des effectifs et des versions.
Elle ne lance ni collecte ni inférence, n’exporte ni texte ni URL et
n’applique aucune suppression ni durée de conservation. Les règles sont :

| Catégorie | Règle | Politique simulée |
| --- | --- | --- |
| `keep` | Verdict courant, pertinent, sans demande de revue | Conserver le texte. |
| `review` | Verdict courant avec demande de revue, quel que soit le verdict | Garder temporairement pour relecture. |
| `discard` | Verdict courant, non pertinent, sans demande de revue | Écarter le texte du stockage permanent. |
| `pending` | Verdict absent, invalide ou d’un autre modèle / d’une autre politique | Attendre un verdict ; ne pas écarter. |

Un verdict valide porte deux booléens explicites (`relevant`, `needs_review`).
L’intégration vérifie ces catégories (dont les verdicts obsolètes et malformés)
sur MongoDB jetable et lance la commande avec un compte qui ne peut pas écrire.

## Résultat sur la base locale

Mesuré le **24 novembre 2026 à 18 h 33 (Paris)** sur les 11 191 documents
présents dans `BlueWhiteThreat.article`. Modèle monté dans `classifier-prd` :
`provisional-oof-advisories-20261020T103652Z`, SHA-256
`1fd5dd5e5531c5b2b80586ae6b1bb23b0f0daa105a232d7f2c76c03fccc36458` ;
politique `source-type-v4`, seuil global de l’artefact `0,488787` sans override.

| Décision simulée | Documents | Part |
| --- | ---: | ---: |
| Conserver | 3 647 | 32,59 % |
| Mettre en revue | 2 004 | 17,91 % |
| Écarter le texte | 5 187 | 46,35 % |
| Attendre un verdict courant | 353 | 3,15 % |

**Décision : laisser la règle en simulation.** Ces proportions mesurent le
comportement des verdicts existants, pas leur exactitude. Elles ne distinguent
pas les vrais et faux rejets. Il faut auditer un échantillon indépendant des
rejets et décider des durées de revue et de conservation avant toute purge.
Le [rapport de test avec les vrais modèles](2026-11-24-pipeline-smoke.md#vérification-locale-avec-les-vrais-modèles)
illustre pourquoi : le rapport CTI synthétique manqué par le classifieur serait
mis en revue, pas écarté. Aucun réentraînement, aucun changement du modèle ou
du seuil, aucune réserve de qualification consommée.
