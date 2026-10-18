# Définition des données CTI — version 1

La politique exécutable est `data/policies/cti-relevance-v1.json`. Elle constitue une définition de travail adoptée sous délégation de l’utilisateur ; elle ne transforme pas les annotations de l’assistant en vérité humaine.

La cible est la pertinence pour une menace cyber concrète ou une défense associée. Le ciblage propre à un client est une dimension distincte. La pertinence d’une revendication ne garantit ni son authenticité ni la faisabilité du service annoncé.

- **1** : incident, vulnérabilité, campagne ou défense liée à une menace identifiée ; revendication explicite de données volées, d’accès compromis ou de prestation cyber malveillante dans le contenu principal.
- **0** : sujet clairement extérieur à cette cible après lecture complète, notamment administration informatique ordinaire ou confidentialité financière sans menace cyber précise.
- **Abstention (`null`)** : origine ambiguë, contexte insuffisant, annuaire, catalogue mixte, page vide ou unité impossible à identifier. Ne jamais convertir automatiquement cette valeur en zéro.

Un domaine Tor, un prix bas, une cryptomonnaie ou le mot « hacker » ne suffisent pas. Une revendication explicite de compromission peut être pertinente même si elle ressemble à une escroquerie. Une page vendeur cohérente reste une unité annotable malgré une navigation de place de marché ; un index mélangeant des annonces doit être segmenté. Les témoignages, menus et mots clés répétés ne remplacent pas le sujet principal.

Chaque annotation conserve l’identité déclarée et le type d’annotateur, la politique et son empreinte, l’empreinte du texte, une justification, les règles appliquées et des citations avec positions exactes en caractères Unicode. Un label décidé exige une déclaration de lecture complète. Une citation négative seule ne démontre pas l’absence d’un sujet dans le reste du document.

`python3 scripts/validate_annotation_contract.py --help` décrit le contrôle et l’export. Par défaut, les annotations d’assistant ne sont pas exportables. L’option explicite d’entraînement sur annotations d’assistant conserve cette provenance et ne contourne pas les contrôles de nouveauté. Les réannotations du train restent bloquées pour une revue de groupe ; le validateur ne propose pas encore de mécanisme automatique pour lever ce blocage.

Ces contrôles vérifient la structure et la liaison aux données, pas la justesse sémantique ni l’identité réelle d’un humain. Ils s’appliquent à ce nouveau chemin d’annotation ; les anciens outils d’entraînement ne les invoquent pas automatiquement.

Exemples fictifs, non utilisés pour entraîner : un tutoriel d’installation d’imprimante est négatif ; une offre revendiquant explicitement un accès volé est positive avec statut « revendication non vérifiée » ; une offre de transfert financier d’origine cachée appelle une abstention.
