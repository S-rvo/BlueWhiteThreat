# Théorie — Extraction d'entités CTI par LLM

> Document pédagogique. Pas un plan d'implémentation (voir `docs/plans/2026-09-14-entity-parser-design.md` et `docs/plans/2026-09-14-entity-parser-implementation.md` pour ça) : ici on explique *pourquoi* le service `parser` est conçu comme il l'est, et *comment* fonctionnent les briques qui le composent.

## 1. Le problème à résoudre

Le crawler et le scraper ramènent du texte brut (articles de blogs sécu, pages darkweb, forums) dans MongoDB. Ce texte contient potentiellement des informations exploitables pour un analyste CTI :

- des **IOCs** (Indicators of Compromise) : adresses IP, hashes de fichiers malveillants, noms de domaine, URLs ;
- des **CVEs** référencées ;
- des **noms de malware** (ex. "LockBit", "Emotet") ;
- des **noms de groupes/acteurs de menace** (ex. "APT28", "Scattered Spider") ;
- des **infrastructures** (serveurs C2, adresses de wallet crypto, etc.).

Aujourd'hui, rien ne fait cette extraction : un analyste humain devrait lire chaque article et noter ces informations à la main. C'est exactement le type de tâche répétitive et à faible valeur ajoutée qu'on veut automatiser (cf. objectif produit : automatiser 80-90% du travail de collecte/enrichissement pour ne garder l'humain que sur la validation, l'attribution et la relation client).

## 2. Pourquoi un LLM plutôt que des règles/regex ?

Deux familles d'approches sont possibles pour extraire ces entités :

**Approche 1 — règles et regex pures.** On pourrait détecter une IP avec une regex, un hash avec une regex, une CVE avec une regex (`CVE-\d{4}-\d+`). Ça marche très bien... mais seulement pour ce qui a un *format fixe*. Un nom de malware ou un nom de groupe APT n'a pas de format : "LockBit 3.0", "Scattered Spider", "UNC3944" ne se ressemblent pas syntaxiquement, et de nouveaux noms apparaissent en permanence. Une regex ne peut pas "reconnaître" un nom de malware qu'elle n'a jamais vu.

**Approche 2 — un LLM lit le texte et comprend le contexte.** Un LLM peut repérer "le groupe connu sous le nom de LockBit a revendiqué..." et en déduire que "LockBit" est un nom de malware/groupe, même s'il ne l'a jamais vu écrit sous cette forme précise dans ce contexte précis. C'est un problème de *compréhension de langage naturel non structuré*, pas de reconnaissance de motif — exactement ce pour quoi un LLM est fait, et pour quoi une regex ne l'est pas.

**Décision retenue : LLM pour tout, puis validation par regex derrière.** On combine les deux forces : le LLM fait le travail sémantique difficile (reconnaître des noms, comprendre le contexte), la regex fait un travail de vérification bête et fiable après coup (est-ce que ce que le LLM a appelé "IP" ressemble vraiment à une IP ?). C'est le principe de la section 4.

## 3. Où s'exécute l'extraction : un service séparé

Le service `parser` est un binaire Go indépendant, au même titre que `crawler`, `scraper`, `api`. Il ne s'insère pas dans la boucle du scraper. Pourquoi les séparer ?

- **Découplage des rythmes.** Scraper du contenu et faire tourner un LLM dessus sont deux activités avec des contraintes très différentes (le scraping est limité par le réseau/Tor, l'extraction LLM est limitée par le CPU/GPU local ou par un budget d'API). Les coupler forcerait l'un à attendre l'autre.
- **Rejouabilité.** Si on améliore le prompt d'extraction ou qu'on change de modèle, on veut pouvoir *re-extraire* les entités sur les articles déjà scrapés, sans avoir à re-scraper. Un service séparé qui lit MongoDB permet ça trivialement.
- **Cohérence avec l'existant.** C'est le style déjà en place dans le repo : chaque étape du pipeline (crawl → scrape → API) est un service Go autonome avec sa propre boucle (`time.Ticker`), qui interroge périodiquement son entrée (Redis pour le crawler, MongoDB pour le parser) plutôt que de réagir à un évènement en temps réel. On garde ce pattern plutôt que d'introduire un mécanisme événementiel (ex. queue Redis dédiée) qui ajouterait de la complexité pour un gain marginal au volume actuel (quelques milliers d'articles/jour).

## 4. Le duo Extraction + Validation

C'est le cœur de la fiabilité du système, et le point le plus important à comprendre :

```
texte brut  →  [LLM]  →  Entities (brut, potentiellement halluciné)
                              │
                              ▼
                     [validation regex]
                              │
                              ▼
                  Entities (propre, garanti bien formé)
```

**Le LLM peut halluciner des formats.** Même un bon modèle peut, sur un texte ambigu, sortir une "adresse IP" comme `192.168.1.999` (octet invalide) ou un "hash" de 20 caractères hexadécimaux (ni un MD5, ni un SHA1, ni un SHA256). Ce n'est pas un bug du modèle, c'est une limite intrinsèque des LLMs sur des tâches d'extraction structurée : ils génèrent du texte plausible, pas du texte garanti-conforme.

**La regex n'a pas ce problème, mais n'a pas non plus l'intelligence du LLM.** Une regex `^(\d{1,3}\.){3}\d{1,3}$` avec vérification que chaque octet est ≤255 est *déterministe* : soit la chaîne est une IP valide, soit elle ne l'est pas, sans ambiguïté et sans coût de calcul. Mais elle ne peut pas décider si "LockBit" est un nom de malware — elle n'a aucune notion de sens.

**D'où la répartition des rôles :**
- Pour les champs à **format vérifiable mécaniquement** (IP, hash, domaine, CVE) : le LLM propose, la regex valide et élimine ce qui ne colle pas au format. C'est un filtre de sécurité gratuit et déterministe contre les hallucinations de format.
- Pour les champs **sans format vérifiable** (noms de malware, noms de groupes, infrastructures en langage libre) : il n'y a pas de regex possible qui ait du sens ("LockBit" et "asldkjqwe" ont la même "forme" — une suite de lettres). On garde donc la sortie du LLM telle quelle ; la validation *sémantique* de ces champs-là (est-ce que ce nom de malware existe vraiment ?) reste hors périmètre de ce chantier, et relèvera soit d'une revue humaine, soit d'un enrichissement externe futur (priorité #3 de la roadmap : croiser avec des bases externes comme MISP).

C'est pour cette raison que `ValidateAndClean` (déjà implémentée, Task 1 du plan) filtre `IOCs.IPs`, `IOCs.Hashes`, `IOCs.Domains` et `CVEs`, mais laisse `Malware`, `ThreatActors`, `Infrastructure` intacts.

## 5. Deux implémentations d'un même contrat : `Extractor`

```go
type Extractor interface {
    Extract(ctx context.Context, text string) (Entities, error)
}
```

Le choix de définir une interface plutôt que d'appeler directement Ollama ou Claude a un objectif précis : permettre de **combiner** ou de **substituer** différents "cerveaux" d'extraction sans toucher au reste du pipeline (validation, stockage). Concrètement, deux implémentations partagent ce contrat :

### 5.1 `OllamaExtractor` — le modèle par défaut, local et gratuit

Ollama fait tourner un LLM open-weight (`qwen2.5:14b-instruct`) directement sur la machine, sans appel réseau externe. Deux conséquences directes :

- **Coût nul et volume illimité.** Pas de facturation à la requête ni à l'article — on peut traiter tout le flux d'articles sans réfléchir au budget.
- **Confidentialité.** Le texte scrapé (potentiellement issu du darkweb) ne quitte jamais l'infrastructure locale.

En contrepartie, un modèle 14B est moins capable qu'un modèle frontière comme Claude sur les cas difficiles — d'où la nécessité d'un filet de sécurité (section 6).

Techniquement, l'appel se fait en HTTP brut vers `POST http://localhost:11434/api/chat`, avec le paramètre `format` renseigné à un JSON Schema représentant `Entities`. C'est le mode "sortie structurée" d'Ollama (supporté depuis la v0.5) : plutôt que de demander au modèle de "répondre en JSON" par instruction dans le prompt (fragile, le modèle peut dévier), on contraint la génération elle-même à respecter un schéma — le modèle ne peut physiquement pas produire un JSON qui ne respecte pas la forme demandée. C'est un premier filet contre les erreurs de *structure* (avant même la validation de *format* de la section 4, qui porte elle sur le contenu des champs, pas leur présence/type).

### 5.2 `ClaudeExtractor` — le filet de sécurité conditionnel

Pour les cas où le modèle local échoue (texte ambigu, jargon rare, structure inhabituelle), on peut escalader vers l'API Claude, plus capable mais payante. La contrainte de conception est : **ne payer que pour la fraction de volume qui en a vraiment besoin**, pas pour chaque article.

Techniquement, l'extraction "forcée" via Claude utilise le pattern *tool use* avec un seul outil disponible (`record_entities`) et `tool_choice` forcé sur cet outil : Claude est obligé d'appeler cet outil avec des arguments respectant son schéma JSON pour répondre, ce qui donne la même garantie de structure que le mode JSON Schema d'Ollama, via un mécanisme différent (adapté au SDK Claude en Go, où l'équivalent direct du "structured output" n'est pas encore documenté).

### 5.3 Pourquoi une interface commune rend ça propre

Parce que les deux extracteurs respectent `Extractor`, le code qui orchestre l'extraction (section 6) n'a pas besoin de savoir lequel des deux il utilise à un instant donné — il voit juste "un `Extractor`, j'appelle `Extract`, j'obtiens des `Entities` ou une erreur". Changer de modèle local, ajouter un troisième extracteur (ex. un autre modèle open-weight) ou changer l'ordre de la stratégie de fallback ne nécessite de toucher qu'un point d'orchestration, jamais la logique métier de validation/stockage.

## 6. La stratégie d'escalade : `ExtractWithFallback`

L'objectif est un **coût quasi nul avec une fiabilité correcte**, ce qui exclut deux extrêmes :
- tout envoyer à Claude (fiable, mais facturé à chaque article — inutilement cher, la majorité des articles sont "faciles") ;
- tout garder sur Ollama sans filet (gratuit, mais on perd silencieusement les cas difficiles que le modèle local rate).

La logique retenue escalade d'Ollama vers Claude **seulement si** :
1. Ollama renvoie une erreur (timeout, réponse mal formée après retries), ou
2. Ollama renvoie un résultat "vide" (`Entities.IsEmpty() == true`) — signe potentiel que le modèle local n'a rien trouvé alors qu'il y avait peut-être quelque chose à trouver.

Dans les deux cas, on retente avec Claude sur ce document précis avant d'abandonner. Ça borne la dépense Claude à une fraction du volume total (les cas difficiles), au lieu d'un coût proportionnel à *tout* le flux. C'est le même principe que "l'escalade au support niveau 2" dans un centre d'appels : le niveau 1 (gratuit, rapide) traite le gros du volume, seul ce qu'il ne sait pas résoudre remonte au niveau 2 (plus cher, plus compétent).

Un point de nuance documenté dans le plan d'implémentation : le design initial envisageait de batcher les escalades Claude (via la Batches API, moins chère, un run par jour) plutôt que d'appeler Claude en synchrone par document. Pour le MVP, l'implémentation fait des appels synchrones par document — plus simple à construire et à tester, quitte à optimiser le coût plus tard si le volume d'escalade s'avère significatif.

## 7. Après extraction : deux bases, deux usages

Une fois qu'on a des `Entities` validées pour un article, elles sont écrites à deux endroits, pour deux besoins différents :

- **MongoDB** — on ajoute un champ `entities` au document `article` existant. C'est la source de vérité "plate" : pour un article donné, quelles entités il contient. Ça sert par exemple à afficher les IOCs d'un article donné dans l'interface, ou à savoir qu'un article a déjà été traité (un document sans champ `entities` = pas encore traité ; un champ `entities` vide = traité, rien trouvé — distinction importante pour ne pas re-traiter en boucle un article sans IOC).

- **Neo4j** — on modélise les entités comme des *nœuds* reliés par une relation `MENTIONED_IN` au nœud de l'article/URL source. C'est une vue "relationnelle" : elle permet des questions qu'une base documentaire type Mongo ne peut pas répondre facilement, comme *"quels acteurs de menace sont mentionnés dans les mêmes articles qu'une infrastructure donnée ?"* ou *"quels articles relient ce malware à ce CVE ?"*. C'est cette vue en graphe qui rendra possible, plus tard, l'analyse de corrélation entre entités (par exemple recouper deux campagnes qui partagent une même infrastructure) — un besoin typique d'analyse CTI que Mongo seul ne couvre pas nativement.

Les deux écritures sont indépendantes et peuvent échouer séparément : un échec Neo4j (nœud non créé) n'empêche pas Mongo d'avoir sauvegardé l'entité, et ne bloque pas le traitement de l'article suivant. On préfère un système qui dégrade proprement (une des deux vues est temporairement incomplète) plutôt qu'un système qui bloque tout le pipeline sur l'échec d'une seule écriture secondaire.

## 8. Gestion des erreurs — principes généraux

Trois règles guident la gestion d'erreurs du service :

1. **Un article ne doit jamais rester "orphelin" silencieusement.** Timeout ou erreur du modèle local → on retente un nombre limité de fois, puis on escalade vers Claude plutôt que d'abandonner l'article sans résultat.
2. **Un échec localisé ne doit pas bloquer le reste du flux.** Un échec de stockage Neo4j sur un article donné est loggé, mais n'empêche pas le parser de continuer sur les articles suivants.
3. **"Rien trouvé" doit être distingué de "pas encore traité".** Un article traité qui ne contenait aucune entité doit être marqué comme tel (`entities` vide, pas absent), sinon le parser le retraiterait indéfiniment à chaque tick, gaspillant du calcul pour un résultat déjà connu.

## 9. Ce que ce chantier ne couvre pas (volontairement)

Pour rester borné, ce chantier s'arrête à "avoir des entités validées, stockées et reliées en graphe". Explicitement hors périmètre, réservé aux priorités suivantes de la roadmap :

- **Scoring/triage automatique** des articles (ex. sévérité, pertinence pour un client donné) — priorité #2, qui *consommera* les entités produites ici.
- **Enrichissement externe** (interroger VirusTotal sur un hash, croiser une CVE avec sa CVSS, vérifier un IOC contre MISP) — priorité #3.
- **Élargissement des sources de collecte** — priorité #4.
- **Affichage front des IOCs** (page `ioc.vue`, aujourd'hui un placeholder vide) — dépend de ce chantier mais n'est pas traité dedans.

Chacune de ces briques suivantes dépend de celle-ci : sans entités extraites et fiables, ni le scoring, ni l'enrichissement, ni un affichage front pertinent ne sont possibles. C'est pour ça que l'extraction d'entités est la priorité #1 de la roadmap.
