# 端到端任务资料

这里保存 `docs/product-evaluation.md` 要求的三十个 PR 任务。Go、Python、TypeScript、Java、Rust 各有两个开源项目、六个任务。`index.json` 列出当前入选项；资料准备完成不表示测试已通过，也不表示完成了 Agent 对比。

## 每项资料

| 文件 | 用途 |
| --- | --- |
| `request.md` | 发给 Agent 的行为请求；公开 API 名称属于调用契约，不包含参考实现步骤 |
| `task.json` | PR 合并证据、实际起点、文件分类、行数、测试命令与资料 SHA256 |
| `reference.patch` | 从合并提交第一父节点到实际合并结果的完整补丁 |
| `implementation.patch` | 参考补丁中的非测试部分，用于单独核对已有测试 |
| `hidden.patch` | 原 PR 的行为测试及必要的补充验收，必须在 Agent 结束后加入 |
| `UPSTREAM-*` | 对应上游许可证及声明 |

非测试源码改动按新增与删除行数之和统计，包含源码注释，排除测试、文档目录、构建脚本、示例和基准目录。Rust 内联测试另行核对，不能仅按 `.rs` 文件统计。每项保留逐文件数字，门槛为二十至三百行。PR 元数据的 `api_base_sha` 仅作来源记录；实际起点使用 `merge_commit` 的第一父提交。

隐藏验收优先使用原 PR 测试。只针对新私有辅助函数的测试不作为唯一实现的要求，排除项列在 `hidden_excluded_files`；补充测试列在 `hidden_authored_files`。这些区别不改变完整参考补丁。重构任务可能在起点就通过行为测试，必须明确标记，测试通过不能证明性能改善。

## 准备 Agent 工作区

先在评估工作区外取得相应公开仓库，并核对 `task.json` 中的对象存在。以下命令只创建一个新的起点工作区，不加入参考实现或隐藏测试：

```sh
python3 eval/scripts/dataset.py eval/dataset/pflag-330 \
  /absolute/path/to/source/pflag /absolute/path/to/new-workspace
```

脚本只导入起点 tree 和 blobs，然后建立一个新的基线提交。工作区没有上游未来提交、远端引用或参考资料，也不会因 `export-ignore` 丢掉起点文件。输出的准备回执保存在工作区外；向 Agent 发送 `request.md` 的内容，不把整个资料目录复制进去。

已存在的目标目录、起点不匹配、资料摘要变化、工作区外符号链接及暂不支持的 tree 条目会使准备失败。Windows 的符号链接权限和文件名边界仍需原生验证，见 `notice.md`。

## 核对起点与参考

`validate_dataset.py` 分别创建起点和参考工作区。每个工作区先准备依赖、记录实际工具版本并运行已有测试，再加入隐藏补丁、运行隐藏验收。参考工作区只在已有测试前应用 `implementation.patch`。这是任务资料验证流程，不运行 Agent，也不计入三百六十次对照。

```sh
python3 eval/scripts/validate_dataset.py eval/dataset/pflag-330 \
  /absolute/path/to/source/pflag /absolute/path/to/new-result \
  --runtime /absolute/path/to/runtime.json
```

`runtime.json` 指定实际工具环境、版本命令与单次命令的秒数上限，例如：

```json
{
  "environment": {"GOTOOLCHAIN": "go1.26.0"},
  "versions": [["go", "version"], ["git", "--version"]],
  "timeout_seconds": 600
}
```

每次结果保留当时的任务资料、脚本、工具输出、命令参数、耗时、退出码、超时标记、锁文件摘要以及两组已有／隐藏测试结果。Python 包元数据放在验证目录的 `site-packages`，工作区 `src` 优先参与导入。命令中的 `{workspace}`、`{variant_directory}`、`{pathsep}` 由脚本替换为当前路径和平台分隔符，不经过 shell 求值。

基线新增能力尚不存在时，可以是预期的编译或断言失败；必须在 `baseline_failure_patterns` 中核对具体行为。超时、准备失败或未识别错误是 unknown。只有起点已有测试、参考已有测试、参考隐藏测试和明确的基线预期都通过核对，才标记该任务有效。跳过的测试仍需从原始输出单独报告，不能计作已运行。

如果只补充基线失败的匹配规则或退出码，可用 `reassess_dataset.py` 重新核对已有证据：

```sh
python3 eval/scripts/reassess_dataset.py eval/dataset/json-1226 \
  /absolute/path/to/result/result.json /absolute/path/to/result/assessment.json
```

复核会比较当时保存的资料与当前资料，并校验每份原始输出的摘要。请求、补丁、起点、命令等执行输入改变时拒绝复用；新记录保留来源结果，原结果不被覆盖。早期探测只保存了日志和输入摘要，不适用这项复核，最终结果使用完整留档。

TypeScript 任务按起点声明安装依赖。Zod 使用 Nub 0.8.3，仍以现有 pnpm 锁文件和 Vitest 运行；Nub 的安装与锁文件兼容性见[官方说明](https://nubjs.com/docs)。Valibot 使用起点声明的 pnpm 11.5.0。工具安装在临时目录，产品没有因此新增对 Nub 验证命令的识别能力。

## 排除与未完成项

`excluded/` 保存实际运行后排除的资料，失败结果继续保留：

| 候选 | 排除原因 | 替代项 |
| --- | --- | --- |
| Cobra #1308 | 旧 golden 固定版权年份为 2021，当前起点与参考均失败 | Cobra #2220 |
| json #1345 | 改变错误位置后须配套更新旧断言，不能独立通过起点的已有测试 | json #1205 |
| Zod #6587 | 当前固定依赖下，参考实现的既有包体积测试测得 4657 字节，超过 4645 字节上限 | Zod #6660 |
| clap #6521 | 延迟初始化后，原有 unstable-v5 帮助测试需要配套增加显式构建 | clap #6413 |

筛选没有修改系统时间、删除旧断言或放宽上限。这些结果说明候选不适合本任务集的独立比较条件，不构成 TaskBelay 的产品缺陷。clap #6454 扣除内联测试后不足二十行，未进入正式任务资料。

2026-10-25，三十项均已完成 macOS arm64 原生资料核验：起点已有测试、参考已有测试和参考隐藏验收的命令通过，基线隐藏验收符合明确预期。逐项结果及摘要见 `../results/2026-10-25-macos/dataset/summary.json`；框架跳过项保留在原始日志，不能计成实际运行通过。这些资料验证未计入 Agent 开启／关闭对照、半自动或接续，不能从参考实现通过推导 Agent 或 TaskBelay 的效果。
