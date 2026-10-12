# MoSCoW — DARKINT (Traque Automatisée d'écosystèmes cybercriminels)

## Must have (indispensable pour le MVP)
- Crawler Tor en collecte continue (planification automatique), découverte de pages depuis des seeds .onion.
- Scraper résilient (gestion du rate-limiting et des protections anti-bot de base) persistant le contenu dans MongoDB.
- Connecteur de collecte Telegram (bot dédié sur les canaux publics identifiés).
- Classifieur ML de triage en boucle de ré-entraînement continue (bootstrap → train → eval → repeat), avec seuil de précision ajustable.
- Dashboard de suivi des métriques du pipeline (volumes collectés, taux de pertinence, dérive du modèle).
- Service d'extraction d'entités et de relations qualifiées (IOC, pseudonymes, TTPs MITRE ATT&CK, liens acteur ↔ wallet ↔ fuite) via LLM local, avec scoring de confiance par entité.
- Base de données en graphe (Neo4j) peuplée en continu, avec interface de visualisation permettant de suivre un acteur dans le temps.
- Connecteur d'export au format STIX 2.1 / TAXII, réinjectable dans un MISP ou un SIEM.
- Alerting automatique sur l'apparition d'un nouvel acteur ou TTP surveillé.
- Au moins deux rapports d'investigation complets illustrant le suivi bout-en-bout d'un acteur ou d'un groupe, et une démonstration devant une équipe SOC ou l'école.

## Should have (important mais pas bloquant)
- Documentation technique complète (architecture, schémas de données, guide de reprise pour une autre équipe).
- Interface web pour consulter les rapports d'investigation générés.
- Tuning fin du scoring de confiance sur l'extraction d'entités (réduction des faux positifs/négatifs identifiés en diagnostic).
- Historique et rejouabilité du graphe (consulter l'état des relations à une date donnée, pas seulement l'état courant).

## Could have (si le temps le permet)
- Connecteur de collecte Element/Matrix.
- Support multilingue renforcé sur l'extraction d'entités (contenu non anglophone).
- Export de rapports d'investigation en PDF formaté automatiquement depuis le graphe.

## Won't have (hors périmètre de ce semestre)
- Interception ou déchiffrement de communications chiffrées de bout en bout.
- Toute fonctionnalité de cartographie ou de ciblage de contenus pédopornographiques ou assimilés — exclu durablement et par principe du périmètre du projet.
- Signalement automatisé aux autorités (PHAROS, etc.) — reste une action manuelle, hors scope outil.
- Déploiement en production chez un client final ou une entreprise tierce.
