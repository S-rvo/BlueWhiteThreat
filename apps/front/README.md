# Interface BlueWhiteThreat

Interface Nuxt. Utiliser Node 22 comme dans Docker, et npm avec le fichier
`package-lock.json` du dépôt :

```sh
cd apps/front
npm ci
npm run dev
```

Le serveur de développement écoute sur `http://localhost:8080`. Le démarrage
avec l’API et les autres services est décrit dans le [runbook](../../docs/runbook-services.md).

Pour vérifier le build sans démarrer les services :

```sh
npm run build
```
