<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="packages/webui/src/assets/taskbelay-wordmark-dark.svg" />
    <img src="packages/webui/src/assets/taskbelay-wordmark.svg" width="360" height="180" alt="TaskBelay" />
  </picture>
</p>

<h1 align="center">TaskBelay</h1>

<p align="center"><strong>La programmation IA au long cours, avec assurage.</strong></p>

<p align="center">
  <a href="README.md">English</a> · <a href="README_zh-CN.md">简体中文</a> · <a href="README_zh-TW.md">繁體中文</a> · <a href="README_ja.md">日本語</a> · <a href="README_ko.md">한국어</a> · <a href="README_es.md">Español</a> · <a href="README_fr.md">Français</a> · <a href="README_de.md">Deutsch</a> · <a href="README_pt-BR.md">Português (Brasil)</a>
</p>

## Ce que TaskBelay vous permet de faire

Votre agent choisit la prochaine action technique.<br />
TaskBelay garde la tâche sous contrôle.

Utilisez-le avec Codex, DeepSeek, Claude Code ou ZCode. L’agent raisonne sur le code ; TaskBelay conserve le périmètre, les limites de vérification, l’état et les données de reprise.

En escalade, la personne qui assure gère la corde tandis que la personne qui grimpe choisit sa voie. TaskBelay reprend cette répartition : l’agent prend les décisions techniques ; TaskBelay vérifie le périmètre approuvé et les limites de vérification, puis utilise les données enregistrées pour reprendre après un échec ou un résultat incertain. Ce n’est ni un bac à sable ni un autre agent de programmation.

- **Périmètre explicite:** Comparez les modifications aux fichiers approuvés. Sortir du plan demande une décision.
- **Vérification bornée:** Prévoyez les contrôles utiles et leurs limites. Tout ajout demande une raison concrète.
- **État persistant:** L’état de référence de la tâche reste enregistré localement entre les sessions.
- **Reprise sûre:** Consultez l’état et les opérations enregistrées pour résoudre un échec ou un résultat incertain avant de réessayer.

Il convient aux travaux sur un dépôt qui s’étendent sur plusieurs sessions ou nécessitent des limites
claires de fichiers et de tests. Pour les questions ponctuelles, les explications de code et les petites
modifications sans suivi persistant, utiliser directement Codex, DeepSeek, Claude Code ou ZCode est généralement plus simple.

## Démarrage rapide

> Installez Node.js `>=24` et le Host choisi. Consultez les versions requises et les plateformes vérifiées dans la [matrice de support](docs/SUPPORT-MATRIX_en.md).

### 1. Installer TaskBelay

Installez TaskBelay, puis suivez les instructions d’activation pour [Codex](docs/CODEX_en.md), [DeepSeek](docs/DEEPSEEK_en.md), [Claude Code](docs/CLAUDE_en.md) ou [ZCode](docs/ZCODE_en.md). Le paquet cible Windows x64 et macOS arm64 ; la validation native de ZCode sur macOS reste à effectuer.

```sh
npm install -g taskbelay@latest
taskbelay
```

Choisissez votre Host parmi les options de l’installateur utilisé. Dans Codex, examinez et autorisez le hook TaskBelay dans `/hooks` ; dans DeepSeek, redémarrez le Profile sélectionné. Dans Claude Code, rechargez les plugins ou ouvrez une nouvelle conversation, puis examinez les permissions demandées.

Dans ZCode, installez et activez le plugin dans Settings → Plugins, puis ouvrez une nouvelle conversation pour activer les hooks. La préparation locale du paquet ne confirme pas son chargement par ZCode.

### 2. Démarrer une tâche

Après l’installation correspondante, envoyez l’un de ces messages dans la conversation du Host :

**Codex**

```text
$taskbelay-codex:taskbelay Ajoutez une limitation de fréquence aux échecs de connexion. Modifiez uniquement les fichiers d'authentification et exécutez au plus 4 contrôles ciblés.
```

**DeepSeek Harness**

```text
/taskbelay Ajoutez une limitation de fréquence aux échecs de connexion. Modifiez uniquement les fichiers d'authentification et exécutez au plus 4 contrôles ciblés.
```

**Claude Code**

```text
/taskbelay-claude:taskbelay Ajoutez une limitation de fréquence aux échecs de connexion. Modifiez uniquement les fichiers d'authentification et exécutez au plus 4 contrôles ciblés.
```

**ZCode**

Sélectionnez `taskbelay` dans le menu `/` → Skills du champ de saisie, puis décrivez la tâche.

```text
Utilisez TaskBelay pour limiter les échecs de connexion. Modifiez uniquement les fichiers d’authentification et exécutez au plus 4 contrôles ciblés.
```

Envoyez ces messages dans la conversation, pas dans un terminal. Précisez l’objectif, les critères
d’acceptation, le périmètre des fichiers et la limite de tests.

La première réponse évalue la demande et propose de travailler directement ou avec TaskBelay. En
choisissant TaskBelay, une nouvelle branche de tâche est créée par défaut depuis le HEAD actuel dans
le répertoire actuel. Confirmez la branche et l’inclusion des modifications non commitées existantes.
Les dépendances, la configuration locale, les fichiers et l’état de l’index sont conservés ; la même
session continue si elle peut accéder à tous les répertoires concernés.

Vous pouvez aussi choisir explicitement la branche actuelle ou un worktree Git dédié. Pour un worktree,
précisez également la source locale ou distante et la branche de départ. Codex ouvre le nouveau
répertoire si l’hôte le permet ; DeepSeek et Claude Code fournissent la commande de redémarrage correspondante.

Un répertoire ne peut accueillir qu’une seule Task active. Les modifications manuelles ou provenant
d’autres outils sont également observées, et changer de branche pendant la tâche suspend le processus.
Les répertoires et branches locaux sont conservés à la fin ; les modifications non commitées doivent
être prises en compte au lancement de la tâche suivante.

Avant l’implémentation, examinez et discutez les exigences, la conception, les tâches, les fichiers prévus et le plan de vérification. Le développement commence après votre accord explicite sur le plan complet. Toute révision du plan ou extension du périmètre des fichiers demande un nouvel accord. Choisir TaskBelay ou un worktree ne remplace pas cet accord.

Pour corriger le nom de la branche d’une Task active, demandez au Host de préparer le renommage dans
TaskBelay, puis renommez la branche d’un seul dépôt sans modifier le répertoire, HEAD, l’index ni les
fichiers. TaskBelay vérifie le résultat et poursuit la même Task. L’annulation n’est possible que si
l’état initial est inchangé. Dans Codex, un lancement non exécuté peut être remplacé explicitement ;
un lancement échoué exige la preuve qu’aucune opération cible n’a été appelée. Inspectez d’abord les
résultats incertains ou les opérations partiellement exécutées.

Les révisions successives du plan conservent toutes les références enregistrées dans la Task, sans
niveau d’archivage ni nombre maximal configuré. Les ressources disponibles et la plage des numéros
de révision restent des limites. Les réponses affichent une première page limitée et le total ;
demandez au Host de parcourir tout l’historique par pages pour revoir les anciennes décisions.
Consultez les [opérations de l’espace de travail](docs/WORKTREE-SOURCES_en.md) et la
[référence des commandes](docs/COMMANDS_en.md) pour les étapes et les limites.

### 3. Reprendre et consulter l’avancement

Après un redémarrage de session, revenez au répertoire d’origine et demandez de poursuivre la tâche.
TaskBelay reprend l’avancement enregistré. Si ce répertoire a disparu ou a été remplacé, la tâche reste
en pause jusqu’à sa restauration ou jusqu’à ce que vous l’abandonniez explicitement.

Dans DeepSeek Harness, incluez `/taskbelay` dans le message demandant de reprendre la tâche.

Dans Claude, rouvrez le répertoire et la conversation d’origine, puis demandez explicitement la reprise de la tâche enregistrée avec `/taskbelay-claude:taskbelay`.

Dans ZCode, rouvrez le répertoire d’origine, sélectionnez la Skill TaskBelay et demandez la reprise de la tâche enregistrée. Si un nouveau répertoire est préparé, suivez les instructions fournies pour ouvrir cet espace de travail.

Ces commandes utilisent le gestionnaire global installé. Pour une installation depuis les sources, utilisez l’entrée correspondante du guide.

```bash
# Consulter les intégrations installées
taskbelay status --host all

# Ouvrir la vue locale des tâches
taskbelay webui start
```

Pour l’installation non interactive, les Profiles DSH personnalisés, les mises à jour, la réparation
et la suppression, consultez la [référence des commandes](docs/COMMANDS_en.md).

## Mascotte de bureau

La mascotte nécessite un Adapter configuré et l’application de bureau installée. L’installation d’un Adapter seul n’installe pas l’application de bureau.

La mascotte affiche plusieurs tâches dans des bulles superposées et ouvre la WebUI de chacune. Elle donne la priorité aux tâches bloquées et passe automatiquement à une tâche inachevée lorsque la tâche actuelle se termine ; vous pouvez aussi épingler une tâche. Vous pouvez personnaliser son apparence, contrôler les animations, modifier sa taille et la démarrer ou l’arrêter indépendamment.

```bash
taskbelay pet start
taskbelay pet stop
```

Les applications de bureau ciblent macOS arm64 et Windows 10/11 x64. Consultez le
[guide de la mascotte](docs/DESKTOP-PETS_en.md) pour l’installation et les commandes, et la
[matrice de support](docs/SUPPORT-MATRIX_en.md) pour les environnements vérifiés.

## Limites d’utilisation

TaskBelay contrôle le déroulement de la tâche, pas les permissions du système. Il n’intercepte pas chaque opération sur les fichiers ni chaque commande shell.

Un worktree dédié sépare les modifications de code. Les processus, le réseau, les identifiants et
les services externes restent partagés avec votre environnement.

Terminer une tâche ne crée pas automatiquement de commit, n’effectue pas de push et ne supprime pas
son worktree. Ces opérations nécessitent une autorisation distincte de votre part.

## Documentation

- **Utilisation :** [Codex](docs/CODEX_en.md) · [DeepSeek](docs/DEEPSEEK_en.md) · [Claude Code](docs/CLAUDE_en.md) · [ZCode](docs/ZCODE_en.md) · [Commandes](docs/COMMANDS_en.md) · [Control Center](docs/WEBUI_en.md)
- **Projet :** [Définition du produit](docs/PRODUCT_en.md) · [Matrice de support](docs/SUPPORT-MATRIX_en.md) · [Sécurité](SECURITY.md)
- **Développement et contributions :** [Index de documentation](MANIFEST_en.md) · [Guide de contribution](CONTRIBUTING.md)

## Communauté

[LINUX DO](https://linux.do/)

## Licence

[Apache License 2.0](LICENSE)
