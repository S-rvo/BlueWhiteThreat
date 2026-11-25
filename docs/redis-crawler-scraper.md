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
  disponibles. L'exploration des planifiés est un balayage Redis borné et
  rotatif : ça ne donne pas un ordre chronologique strict sur tout le backlog.
  Les deux voies respectent la disponibilité de l'hôte. Le rendu synchronise
  celle-ci dans le registre et dans l'index du crawler.
- **À retenter** : panne réseau/timeout temporaire. Le scraper espace les
  cycles de 2, 4, 8, 16 et 32 heures, puis place le sixième échec en quarantaine.
  Ces durées sont des délais minimaux ; l'intervalle du service et la charge
  ajoutent éventuellement du retard. Les pannes de stockage ont leur propre
  retry, sans consommer le budget d'échecs du site.
- **Supprimée / bloquée / en quarantaine** : un HTTP 410 va dans
  `crawler:gone`, les rejets de politique dans un état bloqué, et les
  échecs répétés dans un état mort séparé. Un timeout ou un 404 seul ne
  prouve pas que le service est définitivement indisponible.

Le scraper réserve une URL à la fois par worker, réutilise le pool MongoDB,
n'accuse réception qu'après sauvegarde ou verdict, et peut récupérer une
réservation expirée. Chaque worker démarre Chromium avec son propre contexte
de durée de vie ; chaque URL dispose d'un onglet et d'un délai indépendants.
Un arrêt de Chromium est un problème d'infrastructure, pas un échec du
site. Après trois problèmes d'infrastructure consécutifs, le worker
interrompt son cycle pour éviter de vider artificiellement la file.
Si la file de transfert des liens rendus est pleine,
le contenu est sauvegardé ; le transfert manqué est journalisé et les liens
peuvent être redécouverts lors d'une prochaine visite.

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
5. Pour revenir aux anciens binaires, arrêter les nouveaux workers du
   scraper, attendre l'expiration de leurs réservations, puis démarrer
   l'ancien scraper. Il relit les ensembles historiques, mais n'applique
   pas les nouveaux délais par URL : privilégier le correctif en avant
   si cela doit durer. Le planificateur du crawler conserve les mêmes
   clés durables, sans suppression du backlog.
