# Régression synthétique : document → IOC → graphe (24 novembre 2026)

**Question.** La chaîne transmet-elle un article pertinent jusqu’au graphe,
avec une relation vers son article source, sans doubler l’écriture lors d’un
rejeu ? Ce contrôle porte sur le logiciel, pas sur la qualité des modèles.

```sh
python3 tests/pipeline/smoke.py
```

À lancer depuis la racine. Prérequis : Go 1.24.4+, Python 3 et Docker sur le
poste. La première installation des dépendances Go et des images Docker peut
nécessiter Internet ; le scénario ne visite aucune source réseau externe. Le
script crée deux conteneurs nommés aléatoirement avec ports locaux dynamiques,
sans volume existant ni configuration de production. Il les arrête à la fin.
La CI Pipeline exécute le même test.

| Étape | Vérification |
| --- | --- |
| Ingestion | Deux documents HTML synthétiques, nettoyés par `ExtractMainContent` puis écrits via `SaveScrapedPage` ; une seconde ingestion identique ne crée pas de document supplémentaire. URL source, type et date sont conservés. |
| Classifieur | Le binaire réel en mode `-once` charge [une fixture de poids fixes](../../tests/pipeline/model.fixture.json), pas le modèle de production : un verdict positif, un négatif, versions du modèle et de la politique enregistrées. |
| Parser | Les négatifs, non classés et positifs marqués incertains sortent de la sélection automatique. Une sortie HTTP de remplacement d’Ollama contient une adresse exemple valide et une valeur invalide : seule la première est conservée. Une extraction périmée n’est pas acceptée si le texte a changé. |
| Neo4j | Une relation `(:IP)-[:MENTIONED_IN]->(:Article)` relie l’IOC à l’identifiant Mongo de la source. Effacer le marqueur de projection force un rejeu : toujours une seule relation et pas de seconde extraction. Un second test simule une panne puis une reprise. |
| API | Un compte Mongo en lecture seule est accepté pour lire, rejeté en écriture. La réponse ne contient ni texte brut, ni URL source, ni IOC. Les métriques agrégées reflètent les deux fixtures. |

**Résultat local : passé.** Aucun entraînement, aucun déploiement et aucune
réserve de qualification consommée. Le navigateur, Redis/Tor, le vrai modèle
Ollama, le parallélisme de projection et le remplacement des anciennes
relations après changement de contenu ne sont pas couverts. Avant d’activer
le suivi automatique, il faut notamment versionner l’extraction et réconcilier
les relations du graphe lorsqu’un document est modifié, déclassé ou supprimé.

## Vérification locale avec les vrais modèles

Deux textes synthétiques (un rapport d’incident et un texte jardinage)
passent par la même ingestion, avec le type de source `darkweb_page` mais
sans visite réseau. La base utilisée reste jetable. Le script journalise les
empreintes des modèles ; il ne lit ni la configuration privée des services
ni les corpus existants. Le modèle du classifieur local est copié pour la durée
du test, sans modifier ses poids ni son seuil.

```sh
OLLAMA_HOST=127.0.0.1:11434 ollama pull qwen2.5:14b-instruct
python3 tests/pipeline/smoke.py --real-models
python3 tests/pipeline/smoke.py --real-ollama
```

`--real-models` exige que le classifieur local approuve le rapport sans
demande de revue ; un refus de cette fixture fait échouer le test.
`--real-ollama` conserve les mêmes textes mais charge le classifieur de test
pour contrôler séparément l’extraction et la projection, sans modifier un
verdict réel ni contourner le filtre du parser sur des données réelles.
L’API est testée par le mode CI standard, pas par ces deux modes.

Sur une machine où l’Ollama habituel utilise un contexte élevé, on peut
réutiliser son cache avec une instance de test, sans le reconfigurer :

```sh
# Dans un terminal, si le port est libre ; arrêter ensuite avec Ctrl-C.
OLLAMA_HOST=127.0.0.1:11435 OLLAMA_CONTEXT_LENGTH=4096 OLLAMA_NUM_PARALLEL=1 \
  OLLAMA_MAX_LOADED_MODELS=1 OLLAMA_NO_CLOUD=true OLLAMA_NOPRUNE=true \
  OLLAMA_KEEP_ALIVE=0 ollama serve
# Dans un autre terminal, passer --ollama-port 11435 à l’un des deux modes.
```

**Résultat du classifieur réel le 24 novembre 2026.** Version
`provisional-oof-advisories-20261020T103652Z`, SHA-256
`1fd5dd5e5531c5b2b80586ae6b1bb23b0f0daa105a232d7f2c76c03fccc36458`,
politique `source-type-v4`, seuil `0,488787`. Le rapport CTI est classé
non pertinent (`0,440711`) et marqué à revoir ; le texte jardinage est
rejeté (`0,225057`) sans demande de revue. Le parser sélectionne **0 article**.
C’est un faux négatif sur cette fixture, pas un taux d’erreur terrain ;
aucun réentraînement, seuil modifié, déploiement ni lot de qualification.

**Résultat Ollama / Neo4j le 24 novembre 2026.** Le premier essai avec
l’Ollama habituel (`num_ctx=262144`) a interrompu la connexion au chargement
du modèle. Le test séparé a réussi avec l’instance temporaire ci-dessus
(contexte 4 096, un appel en parallèle) et `qwen2.5:14b-instruct`, digest
`7cdf5a0187d5c58cc5d369b255592f7841d1c4696d45a8c8a9489440385b22f6`.
Ollama a répondu en 10,8 s et le parser a conservé l’adresse de test
`192.0.2.42`. Neo4j contient une seule relation `MENTIONED_IN` vers le bon
article après rejeu, sans deuxième appel au modèle. Ce succès avec la
fixture du classifieur ne lève pas le blocage constaté avec le modèle local.
