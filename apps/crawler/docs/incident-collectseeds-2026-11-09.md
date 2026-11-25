# Incident — collectseeds n'a mis en queue que 1/148 adresse (2026-11-09)

## Constat initial

Run `collectseeds` du 2026-11-09 12:31:15 : 148 adresses `.onion` confirmées
vivantes par la vérification Tor, mais une seule ligne "Added URL to queue"
dans les logs, et `Queue size: 0 -> 1`. 147 adresses rejetées silencieusement,
sans aucune indication de la raison.

## Cause structurelle (corrigée)

`AddURLToQueue` (`apps/crawler/internal/db/redis.go`) retournait un simple
`error`, avec `nil` signifiant indifféremment : URL bloquée (CSAM), détectée
comme ferme de sous-domaines, queue à capacité, doublon déjà
crawlé/en queue, **ou** ajout réussi. Impossible de distinguer ces cas à
partir des logs.

Fix (commit `11dfa88`) : `AddURLToQueue` renvoie maintenant
`(reason string, err error)` avec des constantes exportées
(`QueueReasonAdded`, `QueueReasonBlocked`, `QueueReasonFarm`,
`QueueReasonCapacity`, `QueueReasonDuplicate`). `collectseeds` agrège ces
raisons et log une ligne `Détail: ajoutées=X doublons=Y bloquées=Z fermes=W
capacité=V`, et les compteurs sont aussi persistés dans `crawler:stats`.

## Preuve empirique (run du 2026-11-10, post-fix)

Relance de `collectseeds_service` (6 fichiers deepdarkCTI, 761 adresses
candidates, 150 confirmées vivantes) :

```
Détail: ajoutées=146 doublons=0 bloquées=0 fermes=4 capacité=0
```

À ce moment, `urls_crawled`/`urls_to_crawl` étaient vides dans Redis (suite
à un flush effectué pendant les tests de la remédiation scraper). Dans ces
conditions "table rase", la quasi-totalité des adresses vivantes est
acceptée ; seules 4 sont bloquées comme fermes de sous-domaines (détection
dynamique, seuil = 2 préfixes distincts par hash de backend), 0 par le
filtre CSAM statique, 0 par la limite de capacité de queue.

## Conclusion sur l'incident du 2026-11-09

Le filtre CSAM statique et la détection de fermes se comportent normalement
(taux de rejet ~2.7 % ici, contre ~99.3 % le 2026-11-09). Ni l'un ni
l'autre ne peut donc expliquer, à lui seul, le rejet de 147/148 adresses
lors de l'incident initial. La cause dominante la plus probable est le
chemin **doublon** (`isCrawled` ou `isQueued` déjà vrai) : au moment du run
du 09/11, `urls_crawled`/`urls_to_crawl` n'étaient très probablement pas
vides (contrairement à l'état actuel, vidé depuis par le flush de test), et
la quasi-totalité des 148 adresses y figuraient déjà. Non vérifiable a
posteriori (l'état Redis de ce moment-là a été écrasé), mais c'est
l'hypothèse la plus cohérente avec les deux seules mesures directes dont on
dispose (0 % de rejet doublon sur table rase vs. ~99 % de rejet total avec
état peuplé).

Avec l'instrumentation en place, un futur run anormal (ex: nouveau pic de
rejet) sera désormais directement diagnostiqué dans les logs et dans
`crawler:stats`, sans nouvelle investigation manuelle.

## Anomalie relevée en passant (non résolue, pour référence)

`dynamic_blocked_hashes` (SCARD = 4586) et les clés `farm:prefixes:*`
(30131 clés) ont survécu au flush qui a vidé `urls_crawled`/`urls_to_crawl`/
`urls_scraped`/`crawler:stats` — ce sont des clés distinctes, non couvertes
par ce flush. Le volume (4586 hashs bloqués définitivement) n'a pas été
analysé en détail ; le mécanisme (`apps/crawler/internal/db/farmdetect.go`)
paraît sain sur le run observé (4 fermes sur 150, cohérent avec un taux de
faux positifs faible), mais ce volume accumulé sur la durée totale du crawl
n'a pas été audité pour écarter un éventuel sur-déclenchement.
