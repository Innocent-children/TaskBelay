<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="packages/webui/src/assets/taskbelay-wordmark-dark.svg" />
    <img src="packages/webui/src/assets/taskbelay-wordmark.svg" width="360" height="180" alt="TaskBelay" />
  </picture>
</p>

<h1 align="center">TaskBelay</h1>

<p align="center"><strong>長時間の AI コーディングに、ビレイの支えを。</strong></p>

<p align="center">
  <a href="README.md">English</a> · <a href="README_zh-CN.md">简体中文</a> · <a href="README_zh-TW.md">繁體中文</a> · <a href="README_ja.md">日本語</a> · <a href="README_ko.md">한국어</a> · <a href="README_es.md">Español</a> · <a href="README_fr.md">Français</a> · <a href="README_de.md">Deutsch</a> · <a href="README_pt-BR.md">Português (Brasil)</a>
</p>

## TaskBelay でできること

技術的な判断は、Agent に。<br />
タスクの範囲と進行は、TaskBelay で。

Codex、DeepSeek、Claude Code、ZCode と組み合わせて使います。コードを読み解き、次の技術的な行動を選ぶのは Agent です。

クライミングのビレイでは、登る人がルートを選び、確保する人がロープを管理して墜落を防ぎます。TaskBelay も同じ役割分担です。技術的な判断は Agent に任せ、承認された範囲と検証の上限を確認し、失敗や結果が不明な操作には保存済みの記録を使って対処します。サンドボックスでも、別のコーディング Agent でもありません。

- **明確な範囲:** 実際の変更を承認済みのファイルと照合。計画外の作業には判断が必要です。
- **検証の上限:** 必要な検証と上限を計画。追加する場合は具体的な理由を記録します。
- **永続的な状態:** セッションをまたいでも、ローカルに保存したタスク状態を基準にします。
- **安全な復旧:** 失敗や結果が不明な操作は、保存した状態と記録を確認してから再開や再試行を判断します。

複数のセッションにまたがる作業や、ファイル範囲とテスト量を明確にしたいリポジトリ作業に向いています。
一度きりの質問、コードの説明、進捗保存が不要な小さな変更は、Codex、DeepSeek、Claude Code、ZCode を直接使う方が簡単です。

## クイックスタート

> Node.js `>=24` と利用する Host を先にインストールしてください。必要なバージョンと検証済み環境は[サポート表](docs/SUPPORT-MATRIX_en.md)を参照してください。

### 1. TaskBelay をインストールする

TaskBelay をインストールしたら、[Codex](docs/CODEX_en.md)、[DeepSeek](docs/DEEPSEEK_en.md)、[Claude Code](docs/CLAUDE_en.md)、[ZCode](docs/ZCODE_en.md) の手順に従って利用する Host を有効にしてください。パッケージは Windows x64 と macOS arm64 に対応しています。macOS 上の ZCode の実機検証は未完了です。

```sh
npm install -g taskbelay@latest
taskbelay
```

利用するインストーラーの選択肢から対応する Host を選びます。Codex は `/hooks` で TaskBelay hook を確認して信頼し、DeepSeek は選択した Profile を再起動します。Claude Code はプラグインを再読み込みするか新しい会話を開始し、権限の案内を確認してください。

ZCode では Settings → Plugins でプラグインをインストールして有効にし、新しい会話を開始して Hook を反映させます。ローカルの準備完了だけでは、ZCode がプラグインを読み込んだことは確認できません。

### 2. 作業を開始する

対応するインストールを完了してから、Host の会話に次のいずれかを送信してください。

**Codex**

```text
$taskbelay-codex:taskbelay ログイン失敗のレート制限を追加してください。認証関連のファイルだけを変更し、対象を絞った確認を最大 4 件実行してください。
```

**DeepSeek Harness**

```text
/taskbelay ログイン失敗のレート制限を追加してください。認証関連のファイルだけを変更し、対象を絞った確認を最大 4 件実行してください。
```

**Claude Code**

```text
/taskbelay-claude:taskbelay ログイン失敗のレート制限を追加してください。認証関連のファイルだけを変更し、対象を絞った確認を最大 4 件実行してください。
```

**ZCode**

入力欄の `/` → Skills から `taskbelay` を選択し、タスクを説明してください。

```text
TaskBelay を使ってログイン失敗のレート制限を追加してください。認証関連のファイルだけを変更し、対象を絞った確認を最大 4 件実行してください。
```

これらはターミナルではなく会話に送信します。目標、受け入れ条件、ファイル範囲、テスト上限を具体的に記載してください。

最初の応答では依頼を評価し、直接作業するか TaskBelay を使うかを尋ねます。TaskBelay を選ぶと、
既定では現在のディレクトリで現在の HEAD から新しいタスクブランチを作成します。新しいブランチと、
既存の未コミット変更をタスクに含めるかを確認します。依存関係、ローカル設定、ファイル、ステージ状態を
維持し、すべての対象ディレクトリにアクセスできる場合は同じセッションで続行します。

現在のブランチをそのまま使うか、専用の Git 作業ツリーを作成することも明示的に選べます。専用作業ツリーでは
ローカルまたはリモートのソースと開始ブランチも指定します。Codex は宿主が対応していれば新しいディレクトリを
開き、DeepSeek と Claude Code は必要な再起動コマンドを案内します。

同じディレクトリで実行できる Task は一つです。手動や他のツールによる変更も確認対象となり、タスク中の
ブランチ切り替えは進行を一時停止します。完了後もローカルのディレクトリとブランチを残します。次のタスクを
開始する際には、残っている未コミット変更の扱いを確認します。

実装前に、要件、設計、作業項目、変更予定のファイル、検証計画を確認して話し合います。計画全体への明確な承認後に開発を開始します。計画の修正やファイル範囲の拡大には再承認が必要です。TaskBelay やワークツリーの選択は計画の承認とは別です。

進行中の Task のブランチ名を修正するには、Host に TaskBelay での名前変更の準備を依頼してから、
1 つのリポジトリのブランチ名だけを変更します。ディレクトリ、HEAD、インデックス、ファイルは維持します。
TaskBelay が結果を確認すると同じ Task を続行できます。取り消せるのは元の状態が変わる前だけです。
Codex では未実行の起動を明示的に置き換えられます。失敗した起動には対象操作を呼び出していない証拠が必要で、
結果が不明な場合や一部実行済みの場合は先に状況を確認します。

計画を繰り返し改訂しても、保存済みのベースライン参照はすべて Task 内に保持されます。アーカイブへの移動や設定上の件数上限はありませんが、
利用可能なリソースと改訂番号の表現範囲には制約されます。応答には件数を絞った最初のページと総数が表示されます。
古い判断を確認する場合は Host に履歴全体のページ取得を依頼してください。
手順と制限は[ワークスペース操作](docs/WORKTREE-SOURCES_en.md)と[コマンドリファレンス](docs/COMMANDS_en.md)を参照してください。

### 3. 再開して進捗を確認する

セッションを再起動したら、タスクの元の作業ディレクトリに戻り、続行を明示的に依頼してください。TaskBelay は保存済みの
進捗から再開します。元の作業ディレクトリが消失または置換された場合、復元するかタスクの放棄を明示するまで停止します。

DeepSeek Harness では、再開を依頼するメッセージにも `/taskbelay` を含めてください。

Claude では元の作業ディレクトリと会話を開き、`/taskbelay-claude:taskbelay` で保存済みタスクの再開を明示してください。

ZCode では元の作業ディレクトリを開き、TaskBelay Skill を選択して保存済みタスクの再開を依頼します。新しいディレクトリを準備した場合は、返されたワークスペースの開き方に従ってください。

以下はインストール済みのグローバル管理コマンドです。ソース版では、ガイドに記載された対応する入口を使ってください。

```bash
# インストール済みの連携を確認
taskbelay status --host all

# ローカルのタスク画面を開く
taskbelay webui start
```

非対話形式のインストール、独自の DSH Profile、更新、修復、削除については
[コマンドリファレンス](docs/COMMANDS_en.md)を参照してください。

## デスクトップペット

ペットには、設定済みの Adapter とインストール済みのデスクトップアプリが必要です。Adapter だけをインストールしても、デスクトップアプリは入りません。

デスクトップペットは複数のタスクを重ねた吹き出しで表示し、それぞれの WebUI を開けます。ブロック中のタスクを優先し、完了後は別の未完了タスクに自動で切り替わります。特定のタスクを固定することもできます。外観の変更、アニメーションの制御、サイズ変更、個別の起動と停止に対応しています。

```bash
taskbelay pet start
taskbelay pet stop
```

デスクトップアプリの対象は macOS arm64 と Windows 10/11 x64 です。インストールと操作は
[ペットガイド](docs/DESKTOP-PETS_en.md)、検証済みの利用範囲は[サポート一覧](docs/SUPPORT-MATRIX_en.md)を参照してください。

## 利用上の制限

TaskBelay が管理するのはタスクの進行です。OS の権限は管理せず、すべてのファイル操作やシェルコマンドを遮断する仕組みではありません。

専用の作業ツリーはコードの変更を分離します。プロセス、ネットワーク、認証情報、外部サービスは現在の環境と共有されます。

タスクが完了しても、コードのコミット、プッシュ、作業ツリーの削除は自動では実行されません。これらには別途許可が必要です。

## ドキュメント

- **使い方：** [Codex](docs/CODEX_en.md) · [DeepSeek](docs/DEEPSEEK_en.md) · [Claude Code](docs/CLAUDE_en.md) · [ZCode](docs/ZCODE_en.md) · [コマンド](docs/COMMANDS_en.md) · [Control Center](docs/WEBUI_en.md)
- **プロジェクト：** [製品定義](docs/PRODUCT_en.md) · [サポート一覧](docs/SUPPORT-MATRIX_en.md) · [セキュリティ](SECURITY.md)
- **開発と貢献：** [ドキュメント一覧](MANIFEST_en.md) · [貢献ガイド](CONTRIBUTING.md)

## コミュニティ

[LINUX DO](https://linux.do/)

## ライセンス

[Apache License 2.0](LICENSE)
