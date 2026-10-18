# TaskBelay

[English](README.md) · [简体中文](README_zh-CN.md) · [繁體中文](README_zh-TW.md) · [日本語](README_ja.md) · [한국어](README_ko.md) · [Español](README_es.md) · [Français](README_fr.md) · [Deutsch](README_de.md) · [Português](README_pt-BR.md)

**Controle de engenharia para tarefas reais de programação com IA.**

TaskBelay acompanha Codex, DeepSeek Harness, Claude Code e ZCode. Mantém juntos o objetivo autorizado, as mudanças reais e as evidências de verificação para continuar entre sessões com menos decisões repetidas. O agente continua responsável pelo código e pelo julgamento técnico.

## O que você pode fazer

- Manter objetivos, exclusões explícitas e restrições de interfaces, dados, dependências e projeto durante o desenvolvimento. Analisar mudanças dentro dos arquivos, além dos caminhos. Corrigir desvios dentro da autorização existente e perguntar ao usuário quando a autorização ou uma escolha importante precisar mudar.
- Executar verificações locais autorizadas e salvar códigos de saída reais, entradas, condições e resultados. Declarações, execução e conclusões de aceitação são distintas. Mudanças relevantes invalidam evidências; verificações independentes com escopo justificado podem continuar válidas. Replanejar não zera o custo acumulado.
- Alternar implementação e verificação, revisar requisitos e retomar resultados, tentativas falhas e execuções incertas. O histórico recente tem limites; resultados válidos e custos acumulados são preservados.

As verificações de comandos usam `taskbelay check run` pela ferramenta nativa de processos do host, com suas permissões e ambiente. A WebUI oferece busca de tarefas, filtros por host e estado, seleção de idioma, respostas do usuário e recuperação de execuções. Cada objetivo exige uma verificação de aceitação aprovada; aceitar falhas de regressão existentes não a substitui. A leitura do estado salvo não grava dados de consumo.

As dúvidas técnicas mantêm sua identidade até serem resolvidas; omitir o texto não as apaga. As limitações da entrega vêm dessas decisões salvas. O uso do Host distingue dados não informados, parciais e informados; valores indisponíveis continuam desconhecidos. A ajuda dos métodos inclui exemplos de entrada e saída e tratamento de falhas, sem consultar o código-fonte. Novas branches de Task não herdam um upstream implicitamente.

## Começar

São necessários Node.js >=24 e um agente instalado. O desenvolvimento a partir do código também exige Go >=1.26 e pnpm >=11 <12. Este código contém uma evolução incompatível do contrato de engenharia; pacotes publicados e registros de suporte estável não mudaram. Compile este código para usar o novo contrato. Preserve os dados anteriores e escolha explicitamente outro diretório de dados. Não há migração nem reinicialização automática.

Instalação pública:

```sh
npm install -g taskbelay@latest
taskbelay
```

Compilar e instalar o código atual localmente:

```sh
pnpm install --frozen-lockfile --ignore-scripts
node scripts/taskbelay-local.mjs install --host codex --yes
```

Ative o adaptador seguindo seu [guia](packages/codex/README.md) e peça um resultado delimitado:

```text
Use TaskBelay para limitar tentativas de login com falha. Preserve a API pública de autenticação e os dados existentes; não redesenhe o gerenciamento de contas. Execute os testes existentes relevantes e informe resultados reais.
```

Descreva o resultado e as restrições necessárias. O agente explica a abordagem e trabalha dentro da autorização existente. Ajustes comuns de implementação e verificação não exigem nova aprovação; alterar objetivos ou restrições explícitas exige sua decisão real. Resultados ainda válidos são reutilizados. O consumo é registrado continuamente e limites só se aplicam quando você os define. A WebUI combina uma lista compacta de tarefas com as abas Visão geral, Registros de verificação e Requisitos e consumo. A saída é expandida abaixo da verificação correspondente; respostas, recuperação e arquivamento continuam disponíveis. Atualizar registros lê o conteúdo salvo; Verificar código atual observa novamente as condições reais. Retome com o Task ID salvo no espaço original.

O agente mostra o diretório, a branch e as alterações existentes e usa o espaço escolhido ou claramente indicado pelo contexto. Só pergunta quando há ambiguidade, conflitos ou necessidade de uma nova operação de espaço de trabalho. Criar branches ou worktrees e integrar alterações da origem são operações opcionais que seguem a autorização real.

```sh
taskbelay webui start
```

## Limites

Hooks estruturados cobrem caminhos de escrita suportados; alterações por shell e ferramentas externas são observadas nos pontos de controle. Só verificações executadas por Core têm procedência de execução Core. Entradas e condições declaradas não incluem automaticamente dependências ignored, bancos de dados ou serviços remotos. O raciocínio semântico é explícito, mas não prova correção. TaskBelay não é um sandbox nem outro agente de programação.

O custo de Core é medido; análise, espera e tokens do agente são informados e podem estar incompletos. Tarefas mecânicas pequenas costumam ser mais simples com o agente nativo. Os testes do código demonstram capacidades, não uma redução comprovada de defeitos ou supervisão humana. A conclusão preserva arquivos e não autoriza Git, publicação ou exclusão.

## Saiba mais

[Product](docs/PRODUCT_en.md) · [Commands](docs/COMMANDS_en.md) · [Support and verification](docs/SUPPORT-MATRIX_en.md) · [Codex](packages/codex/README.md) · [DeepSeek](packages/deepseek/README.md) · [Claude Code](docs/CLAUDE_en.md) · [ZCode](docs/ZCODE_en.md)
