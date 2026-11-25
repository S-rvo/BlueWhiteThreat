# File Redis du crawler et du scraper

## États et décisions

- **Planifiée** : entrée dans `crawler:scheduled`. Score Unix inférieur ou égal
  à maintenant = arrivée à échéance ; score supérieur = à venir. Une URL en retard
  est toujours en file et n'a pas échoué pour cette seule raison.
- **Prête** : entrée dans `urls_to_crawl`. Les nouvelles admissions sont
  limitées à 5 000 au total et à 64 par hôte ; le reste est conservé dans
  `crawler:scheduled`. Cette limite ne supprime aucune entrée préexistante.
- **Réservée** : le crawler ou le scraper détient un jeton à échéance. Le
  crawler alterne entre les prêts et les planifiés lorsque les deux sont
  disponibles. Les planifiés ont un index secondaire par hôte : le prochain
  candidat d’un hôte devient admissible après son échéance et son délai de
  politesse. L’index se construit progressivement depuis la file canonique
  `crawler:scheduled`, qui n’est ni vidée ni remplacée. Le balayage borné
  reste disponible pour les anciennes écritures pendant la transition.
  Le rendu synchronise aussi la disponibilité de l’hôte dans cet index.
- **À retenter** : panne réseau/timeout temporaire. Le scraper espace les
  cycles de 2, 4, 8, 16 et 32 heures, puis place le sixième échec en quarantaine.
  Ces durées sont des délais minimaux ; les retries échus sont promus toutes
  les 5 secondes, sans attendre les revisites. Un hôte déjà réservé est
  replanifié après une minute ; une panne d’infrastructure après cinq
  minutes. Ni l’un ni l’autre ne consomme le budget d’échecs réseau du site.
- **Supprimée / bloquée / en quarantaine** : un HTTP 410 va dans
  `crawler:gone`, les rejets de politique dans un état bloqué, et les
  échecs répétés dans un état mort séparé. Un timeout ou un 404 seul ne
  prouve pas que le service est définitivement indisponible.

Le scraper réserve une URL à la fois par worker, réutilise le pool MongoDB,
n'accuse réception qu'après sauvegarde ou verdict, et peut récupérer une
réservation expirée. Chaque worker démarre Chromium avec son propre contexte
de durée de vie ; chaque URL dispose d'un onglet et d'un délai indépendants.
Un arrêt de Chromium est un problème d'infrastructure, pas un échec du
site. Après trois problèmes d'infrastructure consécutifs, le worker fait
une pause de 5 à 60 secondes puis reprend. Il n’y a plus de lot ni de pause
de 30 minutes entre les tâches : sur file vide, il attend de 0,5 à 3 secondes,
et ferme son navigateur après 30 secondes d’inactivité. Une réservation
annulée lors d’un arrêt propre est remise en file sans incrément d’échec.
Un arrêt brutal laisse Redis récupérer la réservation après son expiration.
Une prise sur quatre privilégie l’ancien travail de moindre priorité.
Si la file de transfert des liens rendus est pleine,
le contenu est sauvegardé ; le transfert manqué est journalisé et les liens
peuvent être redécouverts lors d'une prochaine visite.

## Découverte et capacité

- **Collectseeds → Redis** : import facultatif de points de départ depuis des
  catalogues publics ; Compose prévoit une exécution toutes les 24 heures
  avec le profil `seeds`. Une adresse extraite n’est ni une admission Redis,
  ni une preuve que le service répond.
- **Crawler → Redis/Neo4j** : visite de découverte et suivi des échéances.
  Quatre workers par conteneur par défaut (`CRAWLER_WORKERS`, 1–32).
- **Scraper → MongoDB** : rendu continu. Avec les trois proxies de Compose,
  `SCRAPER_WORKERS_PER_PROXY=2` donne six workers Chromium dans un seul
  conteneur. Trois réplicas au même réglage donneraient dix-huit workers sur
  les mêmes proxies, pas trois fois la capacité. Comparer les pages
  sauvegardées par heure, le taux d’échecs et la mémoire avant d’augmenter
  la concurrence. Les tailles des deux files ne sont pas interchangeables.

## Exploitation sur un Mac

- Garder OrbStack/Docker actif et lancer la stack avec `restart: unless-stopped`.
  Docker relance un processus qui sort, sauf s’il a été arrêté volontairement.
  Un état Docker `unhealthy` n’est pas un redémarrage : les workers reprennent
  d’eux-mêmes quand Redis/MongoDB/Chromium redeviennent disponibles.
- Sur un Mac portable branché, voir **Réglages Système → Batterie → Options**
  pour empêcher la veille automatique quand l’écran est éteint
  ([guide Apple](https://support.apple.com/guide/mac-help/set-sleep-and-wake-settings-mchlfc3b7879/mac)). L’écran peut
  rester éteint ou verrouillé. Pendant une vraie mise en veille, Docker et
  les workers ne travaillent pas ; la file Redis permet la reprise au réveil.
- Surveiller `/readyz` et `/metrics` **depuis le réseau Compose** ;
  aucun port scraper n’est publié sur le Mac. Le scraper n’est pas sur le
  réseau Compose général : il rejoint trois réseaux internes pour Tor,
  Redis/MongoDB et Prometheus. Cette segmentation ne remplace pas des
  identités Redis dédiées ni un renderer en conteneur séparé. Le tableau Grafana
  **Crawler et scraper — files Redis** affiche notamment l’âge du plus vieux
  retard, la taille des files et les issues par heure sur 24 h.
- Les snapshots sont locaux par défaut. Pour utiliser un disque séparé,
  définir `BWT_REDIS_BACKUP_PATH` dans le `.env` racine (chemin accessible
  et inscriptible depuis Docker), puis recréer seulement `redis-backup`.
  Tester la restauration sur une instance à part. Un répertoire sur le
  même disque n’est pas une sauvegarde hors machine. Garder la surcharge
  Compose d’un poste, les identifiants et les sauvegardes hors de Git.

## Contrôles de déploiement

1. Confirmer le dernier RDB et son checksum. Ne pas remettre un ancien RDB
   par-dessus la base active pour une perte de quelques clés.
2. Tester le scraper durci avec `--check-browser-sandbox` avant démarrage.
   Ne pas faire tourner l'ancien et le nouveau scraper simultanément : seuls
   les nouveaux workers respectent les jetons de la nouvelle file.
3. Observer `scheduled_due_count`, `scheduled_future_count`,
   `oldest_due_age_seconds`, les hôtes distincts réellement visités et les
   erreurs d'infrastructure. Côté scraper, comparer `ZCARD scraper:pending`,
   `scraper:inflight`, `scraper:retry_due`, `scraper:dead` et
   `scraper:blocked`. Une réservation n'est pas une visite réussie.
4. La règle Redis conserve les suppressions nécessaires aux caches, mais
   interdit notamment `DEL urls_crawled` et `DEL urls_scraped`.
   Vérifier via `ACL DRYRUN`, jamais en lançant réellement la suppression.
   Le fichier ACL s'applique à la création du conteneur avec ce Compose.
   Il protège contre la suppression accidentelle ; il ne remplace ni les
   sauvegardes ni une authentification Redis par service.
5. Ne pas appliquer le changement de réseaux à la stack active pendant un
   déploiement des seuls workers : Redis, MongoDB et Tor devront rejoindre
   les nouveaux réseaux, à planifier dans une fenêtre dédiée. Sur le Mac,
   surveiller aussi la mémoire Redis durant la reconstruction de l’index
   secondaire et valider avec une restauration isolée avant la bascule.
6. Pour revenir aux anciens binaires, arrêter les nouveaux workers du
   scraper, attendre l'expiration de leurs réservations, puis démarrer
   l'ancien scraper. Il relit les ensembles historiques, mais n'applique
   pas les nouveaux délais par URL : privilégier le correctif en avant
   si cela doit durer. Le planificateur du crawler conserve les mêmes
   clés durables, sans suppression du backlog.
