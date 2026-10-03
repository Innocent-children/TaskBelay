# Protocol Fixtures

本目录保存有界、可复查的 public projection fixture。它们由 contract tests 消费，不被
Application 当作 runtime workflow authority。fixture 不包含真实数据库路径、用户目录、source
content、Git diff/status、raw command output、environment、token 或 secret。

## Current graph fixtures

- `graph-server-info.json`：完整、定序的 current ServerInfo、`standard-development`、三种
  method profile、十八工具 catalog 和默认 false/false Host 偏好；
- `graph-multi-repository-open.json`：一个主仓库、一个附加仓库在全部专属 worktree origin 通过核验
  后共享同一 Task、Action、revision 和有效 workspace digest 的 current contract 样例；
- `graph-workspace-lifecycle.json`：当前 `0.8.0` 存储代际下的 worktree origin、relocation、history
  resolution 与显式 abandon 输入样例；
- `graph-open-requirements.json`：创建 graph Task、TaskIntent、REQUIREMENTS Action 和唯一出边；
- `graph-design-action.json`：DESIGN Action 的完整合法出边；
- `graph-invalid-edge.json`：DESIGN 上 undeclared transition 的 closed zero-write error。

这些文件是当前 source contract 的 static fixture evidence；它们不等同于 native Host 或
released package evidence。

## Current Core Host parity fixtures

- `graph-host-parity-codex.json`；
- `graph-host-parity-deepseek.json`。

两份文件只在 `host`、`origin_host` 和 fixture-specific opaque `request_id` 上不同。它们证明 Core
对两个 Host identity 投影相同的 process、node、method、payload、error 和 storage 语义。
DeepSeek fixture 只证明 Host identity/Core parity；这些 fixture 不实现也不声明 DeepSeek Adapter、
Skill、package、native Journey 或产品支持。静态 fixture 也不是任何真实 Host 证据。

## Evidence classification

- 上述 JSON 都是 **static evidence**；
- contract tests 对 exact JSON/schema/order/parity 的执行结果是 **deterministic contract evidence**；
- 无 fixture 可称为 simulated/native Codex、real DeepSeek、registry package 或 released artifact。
