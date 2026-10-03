<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="packages/webui/src/assets/taskbelay-wordmark-dark.svg" />
    <img src="packages/webui/src/assets/taskbelay-wordmark.svg" width="360" height="180" alt="TaskBelay" />
  </picture>
</p>

<h1 align="center">TaskBelay</h1>

<p align="center"><strong>Programação prolongada com IA, com segurança de corda.</strong></p>

<p align="center">
  <a href="README.md">English</a> · <a href="README_zh-CN.md">简体中文</a> · <a href="README_zh-TW.md">繁體中文</a> · <a href="README_ja.md">日本語</a> · <a href="README_ko.md">한국어</a> · <a href="README_es.md">Español</a> · <a href="README_fr.md">Français</a> · <a href="README_de.md">Deutsch</a> · <a href="README_pt-BR.md">Português (Brasil)</a>
</p>

## O que você pode fazer com o TaskBelay

Seu agente escolhe a próxima ação técnica.<br />
O TaskBelay mantém a tarefa sob controle.

Use com Codex, DeepSeek, Claude Code ou ZCode. O agente raciocina sobre o código; o TaskBelay preserva o escopo, os limites de verificação, o estado e os registros de recuperação.

Na escalada, quem dá segurança controla a corda enquanto quem escala escolhe a rota. O TaskBelay segue essa divisão: o agente toma as decisões técnicas; o TaskBelay verifica o escopo aprovado e os limites de verificação e usa os registros salvos para retomar o trabalho após falhas ou resultados incertos. Não é um ambiente isolado nem outro agente de programação.

- **Escopo explícito:** Compare as alterações reais com os arquivos aprovados. Trabalhar fora do plano exige uma decisão.
- **Verificação limitada:** Planeje as verificações relevantes e seus limites. Ampliá-las exige um motivo concreto.
- **Estado persistente:** O estado de referência da tarefa fica salvo localmente entre sessões.
- **Recuperação segura:** Consulte o estado e as operações salvas para resolver falhas ou resultados incertos antes de tentar novamente.

É útil para trabalhos em repositórios que atravessam sessões ou precisam de limites claros de
arquivos e testes. Para perguntas pontuais, explicações de código e pequenas alterações que não
precisam salvar o progresso, usar Codex, DeepSeek, Claude Code ou ZCode diretamente costuma ser mais simples.

## Início rápido

> Instale Node.js `>=24` e o Host que pretende usar. Consulte as versões necessárias e as plataformas verificadas na [matriz de suporte](docs/SUPPORT-MATRIX_en.md).

### 1. Instale o TaskBelay

Instale o TaskBelay e siga as instruções de ativação para [Codex](docs/CODEX_en.md), [DeepSeek](docs/DEEPSEEK_en.md), [Claude Code](docs/CLAUDE_en.md) ou [ZCode](docs/ZCODE_en.md). O pacote destina-se a Windows x64 e macOS arm64; a validação nativa do ZCode no macOS continua pendente.

```sh
npm install -g taskbelay@latest
taskbelay
```

Escolha seu Host entre as opções do instalador utilizado. No Codex, revise e autorize o hook do TaskBelay em `/hooks`; no DeepSeek, reinicie o Profile selecionado. No Claude Code, recarregue os plugins ou abra uma nova conversa e revise as permissões solicitadas.

No ZCode, instale e ative o plugin em Settings → Plugins e abra uma nova conversa para ativar os hooks. Preparar o pacote local não confirma que o ZCode o carregou.

### 2. Inicie uma tarefa

Após concluir a instalação correspondente, envie uma destas mensagens na conversa do Host:

**Codex**

```text
$taskbelay-codex:taskbelay Adicione limite de frequência para falhas de login. Altere apenas arquivos de autenticação e execute no máximo 4 verificações direcionadas.
```

**DeepSeek Harness**

```text
/taskbelay Adicione limite de frequência para falhas de login. Altere apenas arquivos de autenticação e execute no máximo 4 verificações direcionadas.
```

**Claude Code**

```text
/taskbelay-claude:taskbelay Adicione limite de frequência para falhas de login. Altere apenas arquivos de autenticação e execute no máximo 4 verificações direcionadas.
```

**ZCode**

Selecione `taskbelay` no menu `/` → Skills do campo de entrada e descreva a tarefa.

```text
Use TaskBelay para limitar falhas de login. Altere apenas arquivos de autenticação e execute no máximo 4 verificações direcionadas.
```

Envie essas mensagens na conversa, não no terminal. Descreva o objetivo, os critérios de aceite,
o escopo de arquivos e o limite de testes.

A primeira resposta avalia o pedido e pergunta se você quer trabalhar diretamente ou usar o TaskBelay.
Ao escolher o TaskBelay, o padrão é criar um novo branch de tarefa a partir do HEAD atual no diretório
atual. Confirme o branch e se as alterações existentes ainda não commitadas fazem parte da tarefa.
As dependências, configurações locais, arquivos e estado do índice são preservados; a mesma sessão
continua quando consegue acessar todos os diretórios participantes.

Você também pode escolher explicitamente usar o branch atual ou criar um worktree Git dedicado. Para
um worktree, selecione também a origem local ou remota e o branch inicial. O Codex abre o novo diretório
quando o ambiente permite; o DeepSeek e o Claude Code fornecem o comando de reinício correspondente.

Cada diretório admite apenas uma Task ativa. Edições manuais ou de outras ferramentas também são
observadas, e mudar de branch durante a tarefa pausa o processo. Diretórios e branches locais são
preservados ao concluir; alterações não commitadas precisam ser consideradas ao iniciar a próxima tarefa.

Antes da implementação, revise e discuta os requisitos, o projeto, as tarefas, os arquivos previstos e o plano de verificação. O desenvolvimento começa após sua aprovação explícita do plano completo. Alterações no plano ou ampliação do conjunto de arquivos exigem nova aprovação. Escolher TaskBelay ou um worktree não substitui essa aprovação.

Para corrigir o nome do branch de uma Task ativa, peça ao Host que prepare a renomeação no TaskBelay.
Depois, renomeie apenas o branch de um repositório, mantendo o diretório, HEAD, índice e arquivos.
O TaskBelay verifica o resultado e continua a mesma Task. Só é possível cancelar antes que o estado
original mude. No Codex, você pode substituir explicitamente uma inicialização ainda não executada;
se ela falhou, é necessária evidência de que nenhuma operação de destino foi chamada. Inspecione
primeiro resultados incertos ou operações parcialmente executadas.

Revisões sucessivas do plano mantêm todas as referências salvas dentro da Task, sem camada de
arquivamento nem limite configurado de quantidade. Os recursos disponíveis e a faixa dos números
de revisão continuam impondo limites. As respostas mostram uma primeira página limitada e o total;
peça ao Host que leia o histórico completo por páginas ao revisar decisões anteriores. Consulte as
[operações do espaço de trabalho](docs/WORKTREE-SOURCES_en.md) e a
[referência de comandos](docs/COMMANDS_en.md) para ver etapas e limites.

### 3. Retome e acompanhe o progresso

Depois de reiniciar a sessão, volte ao diretório original e peça para continuar a tarefa. O TaskBelay
retoma o progresso salvo. Se esse diretório sumiu ou foi substituído, a tarefa fica pausada até que
você o restaure ou abandone explicitamente a tarefa.

No DeepSeek Harness, inclua `/taskbelay` na mensagem que pede para retomar a tarefa.

No Claude, volte ao diretório e à conversa originais e peça explicitamente para continuar a tarefa salva com `/taskbelay-claude:taskbelay`.

No ZCode, reabra o diretório original, selecione a Skill TaskBelay e peça para continuar a tarefa salva. Se outro diretório for preparado, siga as instruções retornadas para abrir esse espaço de trabalho.

Estes comandos usam o gerenciador global instalado. Para instalações pelo código-fonte, use a entrada correspondente do guia.

```bash
# Consultar as integrações instaladas
taskbelay status --host all

# Abrir a visualização local das tarefas
taskbelay webui start
```

Para instalação não interativa, Profiles personalizados do DSH, atualizações, reparo e remoção,
consulte a [referência de comandos](docs/COMMANDS_en.md).

## Mascote de desktop

O mascote exige um Adapter configurado e o aplicativo de desktop instalado. Instalar apenas um Adapter não instala o aplicativo de desktop.

O mascote mostra várias tarefas em balões sobrepostos e abre a WebUI de cada uma. Ele prioriza tarefas bloqueadas e passa automaticamente para outra tarefa pendente quando a atual termina; você também pode fixar uma tarefa. É possível personalizar a aparência, controlar animações, ajustar o tamanho e iniciar ou parar o mascote de forma independente.

```bash
taskbelay pet start
taskbelay pet stop
```

Os aplicativos de desktop são destinados ao macOS arm64 e Windows 10/11 x64. Consulte o
[guia do mascote](docs/DESKTOP-PETS_en.md) para instalação e controles, e a
[matriz de suporte](docs/SUPPORT-MATRIX_en.md) para a disponibilidade verificada.

## Limites de uso

O TaskBelay controla o fluxo da tarefa, não as permissões do sistema operacional. Ele não intercepta cada operação de arquivo ou comando de shell.

Um worktree dedicado separa as alterações de código. Processos, acesso à rede, credenciais e serviços
externos continuam compartilhados com seu ambiente.

Concluir uma tarefa não cria commits, envia alterações nem exclui o worktree automaticamente. Essas
operações exigem sua autorização separada.

## Documentação

- **Uso:** [Codex](docs/CODEX_en.md) · [DeepSeek](docs/DEEPSEEK_en.md) · [Claude Code](docs/CLAUDE_en.md) · [ZCode](docs/ZCODE_en.md) · [Comandos](docs/COMMANDS_en.md) · [Control Center](docs/WEBUI_en.md)
- **Projeto:** [Definição do produto](docs/PRODUCT_en.md) · [Matriz de suporte](docs/SUPPORT-MATRIX_en.md) · [Segurança](SECURITY.md)
- **Desenvolvimento e contribuições:** [Índice de documentação](MANIFEST_en.md) · [Guia de contribuição](CONTRIBUTING.md)

## Comunidade

[LINUX DO](https://linux.do/)

## Licença

[Apache License 2.0](LICENSE)
