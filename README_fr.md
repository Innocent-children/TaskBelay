# TaskBelay

[English](README.md) · [简体中文](README_zh-CN.md) · [繁體中文](README_zh-TW.md) · [日本語](README_ja.md) · [한국어](README_ko.md) · [Español](README_es.md) · [Français](README_fr.md) · [Deutsch](README_de.md) · [Português](README_pt-BR.md)

**Un contrôle d’ingénierie pour les vraies tâches de développement avec l’IA.**

TaskBelay accompagne Codex, DeepSeek Harness, Claude Code et ZCode. Il conserve ensemble le résultat autorisé, les changements réels et les preuves de vérification pour poursuivre entre sessions avec moins de décisions répétées. L’agent reste responsable du code et du jugement technique.

## Ce que vous pouvez faire

- Maintenir les objectifs, les exclusions et les contraintes d’interface, de données, de dépendances et de conception pendant le développement. Examiner les changements dans les fichiers, pas seulement leurs chemins. Corriger les écarts dans l’autorisation existante et consulter l’utilisateur quand une autorisation ou un choix important doit changer.
- Exécuter les vérifications locales autorisées et conserver les codes de sortie réels, les entrées, les conditions et les résultats. Déclarations, exécution et conclusions d’acceptation sont distinctes. Les changements pertinents périment les preuves ; des vérifications indépendantes au périmètre justifié peuvent rester valides. Replanifier ne remet pas le coût cumulé à zéro.
- Alterner réalisation et vérification, réviser les besoins et reprendre les résultats, les tentatives échouées et les exécutions incertaines. L’historique récent est borné ; les résultats valides et les coûts cumulés sont conservés.

## Commencer

Node.js >=24 et un agent installé sont requis. Le développement depuis les sources nécessite aussi Go >=1.26 et pnpm >=11 <12. Ces sources contiennent une évolution incompatible du contrat d’ingénierie ; les paquets publiés et le support stable restent inchangés. Compilez les sources pour utiliser le nouveau contrat. Conservez les anciennes données et choisissez explicitement un autre répertoire. Aucune migration ni réinitialisation automatique n’est prévue.

Installation publique :

```sh
npm install -g taskbelay@latest
taskbelay
```

Compiler et installer les sources actuelles localement :

```sh
pnpm install --frozen-lockfile --ignore-scripts
node scripts/taskbelay-local.mjs install --host codex --yes
```

Activez l’adaptateur selon son [guide](packages/codex/README.md), puis demandez un résultat délimité :

```text
Utilise TaskBelay pour limiter les tentatives de connexion échouées. Préserve l’API publique d’authentification et les données existantes ; ne refais pas la gestion des comptes. Exécute les tests existants pertinents et rapporte les résultats réels.
```

Décrivez le résultat et les limites. Avant toute réalisation, l’agent présente sa compréhension, la cause, le plan complet, les responsabilités, le périmètre, les vérifications et les limites de coût pour confirmation explicite. Un changement important exige une nouvelle confirmation ; la progression ordinaire conserve l’accord valide. Chaque vérification doit combler une lacune actuelle et les résultats encore valides sont réutilisés. La WebUI affiche le plan, les preuves et le coût, permet la confirmation et l’archivage des tâches terminées et ouvre une tâche par son lien direct. Reprenez avec le Task ID enregistré dans l’espace initial.

```sh
taskbelay webui start
```

## Limites

Les hooks structurés couvrent les chemins pris en charge ; les modifications par shell ou outils externes sont observées aux points de contrôle. Seuls les contrôles exécutés par Core ont une provenance Core. Les entrées et conditions déclarées n’incluent pas automatiquement les dépendances ignored, bases de données ou services distants. Le raisonnement sémantique est explicite, mais ne prouve pas la correction. TaskBelay n’est ni un bac à sable ni un autre agent de programmation.

Le coût Core est mesuré ; analyse, attente et tokens de l’agent sont déclarés et peuvent être incomplets. Les petites tâches mécaniques sont souvent plus simples avec l’agent natif. Les tests des sources montrent des capacités, pas une diminution démontrée des défauts ou de la supervision humaine. La fin conserve les fichiers et n’autorise ni Git, ni publication, ni suppression.

## En savoir plus

[Product](docs/PRODUCT_en.md) · [Commands](docs/COMMANDS_en.md) · [Support and verification](docs/SUPPORT-MATRIX_en.md) · [Codex](packages/codex/README.md) · [DeepSeek](packages/deepseek/README.md) · [Claude Code](docs/CLAUDE_en.md) · [ZCode](docs/ZCODE_en.md)
