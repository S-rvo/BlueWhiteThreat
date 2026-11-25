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
