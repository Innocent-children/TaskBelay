# 桌面宠物使用与形象指南

[中文](DESKTOP-PETS.md) | [English](DESKTOP-PETS_en.md)

桌面宠物显示一个所选 TaskBelay Task 的已保存状态，点击打开对应 WebUI。形象决定可播放的素材，
本地调度器安排待机活动；Task 状态仍由 Core 决定。本文说明获取与启动、任务选择、程序和素材更新、
动作规则、形象制作及常见问题。

## 环境与交付方式

桌面组件面向 macOS arm64（Apple Silicon）与 Windows 10/11 x64，运行时需要至少一个已安装并配置的 Codex、DeepSeek、Claude Code 或 ZCode Adapter 提供 Core。
Swift Package 与应用 metadata 的部署目标为 macOS 14；最低系统实际运行、Developer ID 签名和 Apple 公证尚未完成正式分发验证。
具体已验证范围见[支持矩阵](SUPPORT-MATRIX.md)。

下文 `productRoot` 指产品目录，macOS 默认是 `~/.taskbelay`；设置与形象保存在其中的 `pet/` 子目录。

正式 `taskbelay` npm 包包含 `runtime/darwin-arm64/TaskBelayPet.app` 与
`runtime/win32-x64/TaskBelayPet`，两个平台均携带九类动作、57 个 PNG 帧。
运行已构建的应用无需编译器或 Electron 开发环境。Adapter 仍独立安装并提供 Core。
正式制备由 macOS arm64 构建机编译 Swift、装配锁定的 Windows x64 Electron 运行时，
然后核对应用版本、架构、默认素材和最终解包文件。macOS 使用 ad-hoc 签名；Windows 正式分发签名尚未验证。

## npm 安装与启动

```bash
npm install -g taskbelay@latest
taskbelay install
taskbelay pet start
```

通过统一入口配置至少一个 Adapter；已有配置可以复用。启动器传入 Core 与数据目录参数，
优先使用用户目录中的应用副本，其次使用包内应用。`pet start` 复用已有应用，更新副本使用下述维护命令。

## 本地构建与安装

先按[统一安装入口](COMMANDS.md#多数用户需要的推荐入口)配置至少一个 Adapter。已有配置可直接复用。
在仓库根目录执行下列命令，输出目录必须是仓库外的绝对路径：

```bash
node scripts/build-desktop-pet.mjs --output "/absolute/pet-build"
```

脚本编译 Swift、装配素材与语言资源、进行 ad-hoc 签名并生成 `.tgz`，同时输出 `desktop-pet-build.json`。
它不重新构建 Core，也不执行发布。取得同样由此脚本生成的本地包时，可以直接进入安装步骤。
将下面的 `<local-package>.tgz` 换成构建结果中 `tarball` 字段给出的实际文件名：

```bash
npm install -g "/absolute/pet-build/<local-package>.tgz"
taskbelay pet start
```

本地包与正式制备复用同一平台应用装配。macOS 构建机器需要 Node.js >=24、Xcode >=27 和 macOS SDK >=27；`xcrun swift` 使用当前选中的 Xcode。构建前会检查工具链并输出版本，失败日志包含标准输出和标准错误。

验证本地安装时，已有用户目录应用副本须先按“更新程序与素材”更新，再依次执行 `taskbelay pet start`、`taskbelay pet stop`、`taskbelay pet start`。最后一次启动应显示宠物，并能继续读取任务；仅编译成功不代表应用已连接 Core。启动器和原生应用按[命令参考](COMMANDS.md#安装与本机入口)读取当前运行状态，并核对实际本机服务。

## Windows 本地构建与安装

Windows 本地开发包的构建需要 Go、Node.js 与 pnpm 位于 PATH，并使用仓库规定的工具链版本。Windows 10/11 x64 使用独立桌面实现，功能包含任务选择、状态气泡、托盘、九类动作、形象导入、缩放、拖动、隐藏恢复与独立启停。运行已构建的应用无需 Electron 或编译器开发环境；本地包携带运行时，仍需已配置的 Adapter 提供 Core。

在仓库根目录准备锁定的 Windows 构建依赖，并选择仓库外输出目录：

~~~powershell
npm ci --prefix packages/desktop-pet/windows
node scripts/build-desktop-pet-windows.mjs --output "C:\pet-build"
npm install -g "C:\pet-build\<local-package>.tgz"
taskbelay install --host all --yes
taskbelay pet start
taskbelay pet stop
~~~

将 `<local-package>.tgz` 替换为 desktop-pet-build.json 中 tarball 对应的文件名。该构建装配 Windows 桌面应用，并通过既有构建目标表生成四个 Adapter 的完整 Core 文件和安装包，绑定路径、版本与 SHA256；复制默认九类动作、57 帧并验证解包后的素材和可执行文件。Mac Core 仅交叉编译，不执行 Mac 程序或测试，不执行发布。Windows 采用系统托盘代替 macOS 菜单栏，素材格式、任务语义和六档缩放一致。

本地分发包的 `--host all` 包含 ZCode；其 `action_required` 提示仍需在 ZCode UI 完成插件安装和启用，见 [ZCode 指南](ZCODE.md)。宠物能够读取 Core 不代表 ZCode 模型会话已经可用。

productRoot 默认是 %LOCALAPPDATA%\taskbelay。已安装桌面目录为 productRoot/pet/TaskBelayPet，入口为 TaskBelayPet.exe；优先使用此目录，其次使用包内 runtime/win32-x64/TaskBelayPet。settings.json、appearances/ 与程序目录分开保存。统一入口在程序缺失或文件变化时通过 taskbelay install、upgrade、repair、reinstall 更新程序副本；统一入口先停止需要维护的实例，再暂存并替换程序目录，保留 settings.json 与 appearances/。单独 pet start 不重装已有程序。普通退出和卸载保留这些数据，确认的 delete-data 或 delete-all 清理宠物偏好和导入形象，保留已安装的桌面程序。

Windows 使用受限渲染器、当前用户范围的本地单实例通道与确认消息，不按进程名或仅凭 PID 停止程序。每轮和点击跳转前核对同一 Core 与数据目录；连接失败保留最后记录并标注断连，任务更新与同步时间分别显示。隐藏/睡眠取消读取和动画，恢复后不重放历史完成提示。

Windows 正式分发签名尚未验证。已记录的原生环境与结果见[适配报告](WINDOWS-ADAPTATION.md)，支持范围见[支持矩阵](SUPPORT-MATRIX.md)。

## 更新程序与素材

先更新 npm 包，再通过维护命令更新用户目录中的应用副本：

```bash
taskbelay pet stop
npm install -g taskbelay@latest
taskbelay repair --host codex --yes
taskbelay pet start
```

选择已配置的 Host；DeepSeek 使用 `--host deepseek --profile <name>`。`install`、`upgrade`、`repair`、`reinstall` 比较包内与已安装应用的文件，缺失或内容变化时才包含宠物更新；同版本本地构建也按内容判断。应用相同时不因插件已就绪而停止或复制宠物。命令入口自更新负责安装新包携带的桌面程序。计划在确认前列出实际需要的操作。更新先停止宠物，再暂存并替换应用目录；停止或复制失败会中止维护，暂存失败保留原应用。设置和导入形象保存在独立目录中。

菜单的运行状态由平台验证：macOS 核对内核启动身份、可执行路径和用户，Windows 通过实例令牌确认本地管道响应。仅有 runtime.json 不代表正在运行；无法验证时显示未知，读取状态不启动或停止程序。

单独安装 npm 包只更新包内应用；已有用户目录副本通过上述维护命令更新。更新形象时从原始文件夹重新导入，程序更新不会补造缺少的动作。

## 默认形象与外部素材

宠物包仅内置蓝色海洋精灵“嘟嘟”，来自 `packages/desktop-pet/default-appearance/`，包含九类动作、57 个 PNG 帧。构建时复制素材并逐文件核对内容。其他自定义形象作为独立素材包，通过“导入形象…”选择包含
`pet.json` 的文件夹导入；支持的格式见下文“形象类型与动作可用性”。

导入后的素材保存在 `productRoot/pet/appearances/<id>`，程序更新保留这些素材。
更新自定义形象时重新导入同一文件夹；切换形象不会改变关注任务。

## 任务选择与基本操作

macOS 菜单栏入口使用 TaskBelay 圆形 TB 标志，以 18 pt 单色矢量显示，由系统根据菜单栏外观和选中状态着色。Windows 托盘使用同款 TB 标志，配浅黄绿色背景，与 WebUI 应用图标一致。

默认以最多三层叠加气泡显示任务：前层显示重点任务，后层露出其他任务的卡片轮廓。底部显示未完成与受阻数量；超过三个时显示更多数量。悬停或点击数量入口展开可滚动的气泡列表，每个任务都可单独打开 WebUI。

没有可展示的任务卡片时，气泡和数量入口隐藏，悬停也不会显示空提示。任务出现后气泡自动恢复。隐藏区域不拦截鼠标，点击角色仍可打开任务列表；完成提示、固定关注和断连时保留的任务卡片仍按下述规则显示。

每轮观察都会查找未完成任务。自动模式优先关注受阻任务，同优先级保持当前对象；需要选择新对象时按最近更新时间确定。重点任务完成后提示约 3 秒，再切换其他未完成任务；本次连续观察到的完成结果保留在展开列表中，打开或关闭提示后移除。取消、归档或确认不可用的任务也会让出自动关注位置。没有其他任务时进入待机。

“固定关注”按当前数据目录保存，固定对象完成后仍可继续查看，其他任务气泡照常显示；“恢复自动”解除固定。普通点击只打开任务，不会固定。重启仅恢复明确固定的任务，本次完成提示不会跨重启保留。

观察间隔为 macOS 每轮结束后 5 秒、Windows 3 秒；完成提示到期会提前刷新。隐藏或睡眠时暂停。读取失败保留最后记录并标注断连；恢复、唤醒及首次读取历史完成任务均不重播庆祝。

普通 Codex 对话不会自动成为 TaskBelay Task。所选任务持续处于进行中时，即使 Host 暂时没有输出，宠物仍展示工作或审核动作。

| 操作 | 结果 |
| --- | --- |
| 左键点击宠物或气泡 | 宠物打开重点任务；各气泡打开自己的 Task WebUI，无重点任务时宠物打开列表。 |
| 右键／菜单栏 | 固定关注、恢复自动、选择形象、导入形象、调整宠物大小、控制动画与待机活动、隐藏或退出。 |
| 鼠标悬停 | 展开多任务气泡列表；符合休闲条件时，角色悬停还可触发挥手。 |
| 拖动后放下 | 保存新的摆放位置，后续散步围绕该位置进行。 |
| 隐藏或系统睡眠 | 停止动画、位移和观察请求；重新显示或唤醒后读取当前状态。 |
| 退出／`taskbelay pet stop` | 只结束宠物，保留 WebUI、Task、设置和素材。 |

命令仅支持 `taskbelay pet start` 和 `taskbelay pet stop`，输出为纯文本，退出码为成功 `0`、运行失败 `1`、参数错误 `2`。
没有公开的 `pet status` 或 `pet start --json`；启动失败时结合错误文字和本指南的常见问题处理。

## macOS 动效

气泡展开与收起使用系统原生动画，快速移入、移出时从当前画面继续变化。卡片和滚动区域保持连续，
展开后仍可点击任务、固定关注和滚动。macOS 26 及以上使用系统 Liquid Glass 材质，macOS 27 使用原生
交互玻璃反馈；较早的受支持系统使用对应的 AppKit 材质与动画。

散步缓缓起步、到达前减速，整体距离和时长遵循下方的待机规则。鼠标进入、按下或菜单打开会中断散步。
关闭“动画”或开启系统“减少动态效果”后，气泡直接切换到目标布局，角色显示静态帧；隐藏与睡眠停止播放。

气泡会利用当前整段动作共同的顶部透明空间靠近角色，保留细节余量；切换帧时位置保持稳定，宠物大小和落脚位置保持不变。

角色按素材原定时长播放，并随屏幕刷新更新画面。主线程短暂繁忙后会显示当前时间对应的帧；
它改善播放计时，不会增加素材本身没有的姿态，低帧数图集仍受原始素材限制。

## 宠物大小

“宠物大小”提供 50%、75%、100%、125%、150%、200% 六档，默认 100%，保存为 `settings.json` 的 `scale`。缩放只改变角色，气泡文字和宽度保持不变。
调整时以素材锚点保持角色位置，再将窗口约束到屏幕可见区域；新大小和位置保存成功后应用，保存失败时显示提示并保持原大小。行走速度按缩放比例调整。

Windows 调整大小时会结束当前待机活动、停止对应位移，再按正常间隔安排下一次活动；正在播放的工作动画和完成庆祝保留播放进度。

## 形象类型与动作可用性

规则按动作键运行，适用于所有形象，不按名称指定行为。素材实际提供哪些动作，决定该形象能展示哪些效果。

| 形象类型 | 保存与播放的内容 |
| --- | --- |
| 单张 PNG 或 SVG | 五类任务动作共用一张图，表现为静态形象。 |
| TaskBelay 原生 PNG/SVG 动画包 | 必须提供五类任务动作，可增加四类附加动作；每类帧数由自己的清单决定，SVG 保留矢量表示。 |
| Codex 标准图集或采用相同布局的高分辨率扩展 | 按固定九行动作提取九类、57 帧。格式 2 的额外追视帧不在提取范围内。 |

五类必需动作是 `idle`、`working`、`blocked`、`complete`、`disconnected`，保证任务状态都有对应展示。
四类附加动作是 `running-right`、`running-left`、`waving`、`review`，用于散步、回应和思考。
缺少的附加动作会从休闲候选中排除；活动任务使用 working。
安装副本的动作可在 `productRoot/pet/appearances/<id>/animations.json` 的 `clips` 中查看。

## 导入与切换

1. 从宠物右键菜单或菜单栏打开“选择形象”。
2. 点击“导入形象…”，选择包含 `pet.json` 的文件夹。
3. 导入成功后立即使用；菜单可以切回内置形象或其他已导入形象。

导入会保存一份副本；源文件夹可以移走。修改素材后重新导入，同 ID 更新已有形象。
转换后的完整形象包在临时目录通过与加载时相同的校验后，才替换已安装目录；验证失败保留原有形象和选择。
任务选择与形象选择分别保存；切换形象不会改变关注任务，也不会重新播放旧的完成提示。

用户素材位于 `productRoot/pet/appearances/<id>`，`selected_appearance` 保存在宠物设置中。
程序升级和普通卸载保留素材与选择；已确认的 delete-data 或 delete-all 清理素材与选择，保留程序。保存的形象缺失或损坏时，
程序提示并暂用内置形象，用户可以重新导入。

## TaskBelay 静态形象

最小形象包只需要描述文件和一张 PNG 或 SVG。以下示例使用 PNG：

```text
orange-square/
  pet.json
  pet.png
```

`pet.json`：

```json
{
  "id": "orange-square",
  "name": "Orange square",
  "image": "pet.png"
}
```

`id` 为 1–64 个小写英文字母、数字、连字符或下划线，首字符为字母或数字；`name` 为 1–100 个字符。
`image` 是相对当前文件夹的 PNG 或 SVG 路径，建议使用透明背景。单张图用于所有阶段，阶段区别仍由气泡文字说明。
可以复制仓库中的 [静态示例](../packages/desktop-pet/examples/orange-square/pet.json)，替换图片与名称。

## TaskBelay 动画形象

动画包的 `pet.json` 只填写 `id` 与 `name`；同目录提供 `animations.json` 和 `Assets/`：

```text
my-pet/
  pet.json
  animations.json
  Assets/
    idle/0.png
    idle/1.png
    working/0.png
    blocked/0.png
    complete/0.png
    complete/1.png
    disconnected/0.png
```

帧可使用 PNG 或 SVG；SVG 文件满足下文“文件要求”，导入后保留原始矢量内容。下面清单使用 128×128 PNG，所有帧使用相同画布和角色位置：

```json
{
  "canvas": { "width": 128, "height": 128 },
  "anchor": { "x": 64, "y": 128 },
  "clips": {
    "idle": { "frames": ["idle/0.png", "idle/1.png"], "fps": 8, "loop_range": [0, 1], "rest_frame": 0 },
    "working": { "frames": ["working/0.png"], "fps": 8, "loop_range": [0, 0], "rest_frame": 0 },
    "blocked": { "frames": ["blocked/0.png"], "fps": 8, "loop_range": [0, 0], "rest_frame": 0 },
    "complete": { "frames": ["complete/0.png", "complete/1.png"], "fps": 8, "loop_range": null, "rest_frame": 1 },
    "disconnected": { "frames": ["disconnected/0.png"], "fps": 8, "loop_range": [0, 0], "rest_frame": 0 }
  }
}
```

五类任务动作均需存在，可以重复引用同一张图。清单还可提供 `running-right`、`running-left`、`waving`、
`review` 四类附加动作；所有已提供动作均按相同规则校验和加载。单帧动作使用静态显示。多帧动作按 `fps` 播放；
也可以添加与 `frames` 等长的 `frame_durations_ms` 数组，逐帧指定毫秒时长并优先于 `fps`。
`loop_range` 包含首尾索引；完成动作使用 `null`，播放结束保持 `rest_frame`。减少动态效果与动画开关使用静态帧。
播放器按完整画布等比缩放，作者用一致画布对齐角色；`anchor` 保留共同素材参考点。

## Codex 标准宠物格式兼容

TaskBelay 兼容 Codex 精灵图格式 1 和 2。兼容范围是本地包中的图集布局、所需动作帧和逐帧时长；
任务状态、提示和点击跳转由 TaskBelay 决定。导入入口接受以下字段组成的 `pet.json` 与 PNG/WebP 图集：

```json
{
  "id": "my-codex-pet",
  "displayName": "My Codex pet",
  "description": "My companion",
  "spriteVersionNumber": 2,
  "spritesheetPath": "spritesheet.webp"
}
```

选择具体宠物文件夹，例如 `~/.codex/pets/my-codex-pet`；该文件夹须包含 `pet.json` 和其引用的 PNG/WebP 图集。
本导入器要求 `id`、`displayName` 和 `spritesheetPath`：`id` 非空且最多 256 个字符，`displayName`
包含非空白内容且最多 100 个字符，`spritesheetPath` 为相对文件路径。`description` 可选，不参与任务展示。
`spriteVersionNumber` 可省略，省略时按 1 处理；显式取值只接受 1 或 2。这与
[Codex 官方宠物版本参数](https://developers.openai.com/codex/reference/commands#pets)一致。

| Codex 标准格式 | 图集尺寸 | 网格 | 单格尺寸 |
| --- | --- | --- | --- |
| `spriteVersionNumber: 1` | 1536×1872 | 8 列、9 行 | 192×208 |
| `spriteVersionNumber: 2` | 1536×2288 | 8 列、11 行 | 192×208 |

两种标准格式共用下面九个动作行，TaskBelay 全部导入，共 57 帧。格式 2 另外包含追视帧，
当前导入范围为前九行动作。
这里的 1、2 是 Codex 素材格式版本，不是 TaskBelay 产品版本。

| 动作 | 清单键 | Codex 动作行 | 帧数 |
| --- | --- | --- | --- |
| 待机 | `idle` | idle，行 0 | 6 |
| 向右走 | `running-right` | running-right，行 1 | 8 |
| 向左走 | `running-left` | running-left，行 2 | 8 |
| 挥手 | `waving` | waving，行 3 | 4 |
| 完成 | `complete` | jumping，行 4 | 5 |
| 未连接 | `disconnected` | failed，行 5 | 8 |
| 受阻 | `blocked` | waiting，行 6 | 6 |
| 活动任务 | `working` | running，行 7 | 6 |
| Idle thinking | `review` | review，行 8 | 6 |

导入时拆成 PNG 帧并保留标准逐帧时长，之后使用统一播放器。向右走、向左走、挥手和 review
参与下述待机活动与任务动作选择；循环素材也可按指定完整遍数播放后结束。
任务含义、完成提示条件和跳转目标继续由 TaskBelay 决定；`behavior-map.json` 不参与导入或动作调度。
源文件夹保持原样，安装目录 ID 从 Codex 的 `id` 稳定生成。

## TaskBelay 自有高分辨率扩展

同一入口也支持 TaskBelay 自有的高分辨率图集扩展。它沿用上面的 `pet.json` 字段和动作行布局，
导入时保留原始单格分辨率，转换为统一 PNG 帧。该扩展由 TaskBelay 支持，不属于 Codex 标准格式；
TaskBelay 导入成功不表示原图集能被 Codex 识别。需要同时在 Codex 中使用时，应另行提供上表中
与 `spriteVersionNumber` 匹配的标准尺寸图集。

扩展切图规则：

- 图集固定 8 列，宽度须能被 8 整除；`cell_width = width / 8`。
- 单格保持 192:208 比例，`cell_height = cell_width × 208 / 192` 必须为整数。
- 高度至少覆盖前 9 个完整动作行，即 `height >= cell_height × 9`；完整提取上表九类动作。
- `spriteVersionNumber` 仍只接受 1、2，省略默认 1；扩展图集的切图尺寸由宽度推导，
  该字段不要求扩展图集具有标准格式的总高度。

例如 12288×14976 的 8 位图集使用 1536×1664 单格，包含 9 行；即使字段填写 2，TaskBelay 也按扩展
规则提取九类动作。它不是 Codex 格式 2 的标准图集。源图集的 RGBA 解码预估为 702 MiB，
最大的 8 帧动作预估为 78 MiB，均在以下限制内；导入还需满足转换后 PNG 总大小限制。

## 待机活动与动作触发

菜单提供默认开启的“待机活动”开关，保存为 `idle_activities_enabled`。关闭后保留基础待机与任务动画，
停止休闲轮换、互动挥手和自动位移。“动画”总开关与系统减少动态效果仍优先，关闭动画时只显示静态帧。

服务连接正常、窗口可见且没有重点任务时进入休闲。自动模式完成提示结束后继续关注未完成任务；固定任务完成、取消或归档后，先保持对应姿态 3 秒再休闲；
真正观察到完成时，先完整播放一次庆祝，再等待这 3 秒。首次读取已完成任务使用静态完成帧，不重播历史庆祝。
任务进行中、受阻、不可用、状态未知或服务失联时使用对应任务展示。

| 动作 | 触发规则 |
| --- | --- |
| 基础待机 | 每个短动作结束后等待随机 6–12 秒，再选择下一项。相同状态的普通轮询保持当前动作和倒计时。 |
| 向右走、向左走 | 初始权重各 25%。对应方向至少有 40 pt 空间时，水平移动 40–80 pt，平均速度约 `20 × scale` pt/秒，持续时间为距离除以速度，100% 大小时为 2–4 秒。结束后有 30% 概率接一次挥手。 |
| 挥手 | 随机权重 20%；空闲启动或重新显示、角色悬停至少 0.4 秒、拖动放下也可触发。每次播放 1–2 遍，共享 20 秒冷却。持续悬停只回应一次，离开至少 2 秒后可重新触发。 |
| Review／看电脑思考 | 随机权重 30%；空闲时随机选择 3–5 秒目标时长，播足完整遍数后回到待机。活动任务统一使用 working；review 仅表示空闲动画，不代表 Core 阶段。 |

先排除缺少素材、冷却中或空间不足的动作，再按剩余权重选择；有其他候选时避免连续重复同一动作。
未提供 review 素材的形象不参与空闲思考候选。空闲思考保留原气泡文字，任务含义和跳转目标继续由 Core 状态决定。

自动散步以最后手动摆放位置为中心，水平范围为左右各 120 pt，目标距当前屏幕边缘至少 16 pt，
完整窗口保持在可见区域内。自动位移只保存在内存中，手动拖动才更新保存位置；屏幕布局变化会取消当前路线并重新约束位置。

鼠标进入宠物窗口会停止位移并暂停随机活动；只有角色区域的悬停触发挥手，气泡悬停仍用于展开信息。
按下鼠标、拖动、打开菜单或选择/导入面板时暂停休闲，左键打开 WebUI、右键打开菜单的行为保持不变。
任务受阻、完成或失联的观察结果立即中断休闲。隐藏、睡眠、关闭相关开关时取消休闲计时和位移，恢复后重新安排，
不补播被中断的动作。动作执行、短动作结束和随机等待分别由本地调度器、播放器与窗口协调，普通轮询不会反复重启动作。

## 文件要求

- 描述文件最多 256 KiB。文件路径必须为相对路径，引用普通文件；不接受符号链接或目录外引用。
- TaskBelay PNG/SVG 动画帧尺寸须与 `canvas` 一致，宽高须为正整数。静态 PNG 和每个 PNG 帧的解码预估最多 128 MiB。
- 动作清单最多 512 个帧引用；引用的 PNG/SVG 素材文件总大小最多 128 MiB，以容纳九类高分辨率动作。每个动作按
  `canvas.width × canvas.height × 4 × frame_count` 估算的 RGBA 数据不超过 128 MiB，`frame_count` 为动作帧数。
- 每个 SVG 最多 1 MiB，采用 UTF-8；根画布宽高为 1–4096 的整数，提供 `viewBox` 时必须为 `0 0 width height`。仅接受受限静态图形、渐变及文件内引用；元素最多 4096 个、嵌套最多 64 层。脚本、嵌入位图、外部引用、DOCTYPE 和实体会被拒绝，当前系统须能渲染素材。
- `fps` 为 0.1–120，逐帧时长为 9–60000 毫秒。循环及静态索引必须有效。
- Codex 标准图集与 TaskBelay 扩展图集的源文件最多 512 MiB，解码预估最多 1 GiB。
- 解码预估在分配像素前按图片元信息计算：`width × height × 4 × ceil(bit_depth / 8)`；`bit_depth` 为位深，支持 1–16 位，
  以四通道估算。它限制图片像素数据，不表示整个进程的内存峰值。
- 源图集满足限制后，转换结果仍须满足 PNG 总大小、动作内存和播放清单限制。
  尺寸超限、整数溢出风险或转换结果超限均以导入错误返回，已安装形象保持原样。

形象包只提供数据，不提供执行脚本。

## 常见问题

| 现象 | 检查与处理 |
| --- | --- |
| 安装 Adapter 后没有宠物应用 | 安装 `taskbelay@latest`，运行 `taskbelay install` 配置 Adapter 后执行 `taskbelay pet start`。 |
| 更新程序后仍是原来的表现 | 更新 npm 包后运行 `taskbelay repair` 更新用户目录副本；程序更新与形象重导入是两步不同操作。 |
| 其他形象没有散步、挥手或思考 | 查看安装副本的 `clips` 是否包含对应附加动作。只有五类动作的形象可正常展示任务，但不能播放未提供的素材。 |
| 源图有九类，安装副本只有五类 | 确认运行的应用副本也已更新，再从包含完整图集的原始文件夹重新导入。程序只读取最近一次导入保存的帧，不会因升级自动补齐。 |
| 是否所有包都有九类、57 帧 | 该固定数量只适用于 Codex 布局图集；单张 PNG/SVG 与原生动画包按上表各自的规则处理。 |
| 一直待机或不散步 | 检查任务选择、服务连接、“动画”和“待机活动”开关、系统减少动态效果，以及素材是否有行走动作。鼠标留在窗口内或菜单／面板打开时会暂停；离开后等待 6–12 秒。某方向不足 40 pt 空间时不会向该方向走。 |
| 悬停后没有再次挥手 | 需满足休闲条件和 20 秒冷却，悬停角色至少 0.4 秒；持续停留只回应一次，移开至少 2 秒后才可重新触发。 |
| 正在聊天却没有工作动作，或没聊天却在工作 | 宠物读取所选 TaskBelay Task 的保存状态，不读取普通 Codex 对话活动或键盘输入。先检查是否选择了正确的 Task。 |
| 选择已完成任务没有跳跃 | 庆祝只在持续观察同一任务从未完成变成 DONE 时触发一次；首次读取完成任务使用静态帧，停留后可进入休闲。 |
| 图集文件没有超限，仍提示 PNG 总大小或内存超限 | 源图集和转换后的帧分别计限。WebP 转 PNG 可能增大文件；检查源图集 512 MiB、解码预估 1 GiB、转换后 PNG 总计 128 MiB，以及单动作 128 MiB 限制。 |
| 显示未连接或启动失败 | 确认平台及 Adapter 配置；已启动时可用菜单“重试连接”。显式 `pet start` 可按需启动 WebUI，后台观察只读。不要用不存在的 `pet status` 排查。 |
| 同目录的 behavior-map.json 没生效 | 当前读取 `pet.json`、图集或 `animations.json`；`behavior-map.json` 不控制帧时长或行为，使用当前支持的清单字段。 |

## 验收方式

按受影响组件与环境选择检查，单独保存产物身份、执行命令、实际结果及未执行项。

| 检查 | 环境与预期结果 |
| --- | --- |
| `swift test --package-path packages/desktop-pet/macos --filter 'PetMotionTests\|PetAnimationTimelineTests\|AnimationAndBubbleTests\|PetTaskCollectionTests\|PetActivityTests'` | macOS；原生窗口的动画反向、中断、屏幕约束、滚动与按钮命中，以及逐帧时间线与活动规则；窗口检查使用构造的任务数据 |
| `swift test --package-path packages/desktop-pet/macos --filter TaskObserverTests` | macOS；使用模拟 Core/HTTP 检查多任务发现、固定关注、自动交接与迟到响应处理 |
| `node --test packages/taskbelay/tests/pet.test.mjs` | 使用源码编译的真实 Core 检查无服务时的状态输出、服务启动与启动器读取；原生窗口交接使用模拟入口。需要 Go。 |
| `swift test --package-path packages/desktop-pet/macos --filter WebUIModelTests` | macOS；除模型测试外，编译并调用真实 Core，检查状态解码及原生启动参数。需要 Go；最终应用的显示与启停另行验证。 |
| `swift test --package-path packages/desktop-pet/macos --filter PetAppearanceTests` | macOS；静态、原生和 Codex 素材导入保留支持的帧与时长，失败时保留已安装形象 |
| `node --test packages/desktop-pet/windows/tests/contracts.test.cjs` | Node.js；检查 Windows 素材加载与导入的帧数和时长限制，超限重导入保留已安装形象；不运行 Windows 原生窗口 |
| `node --test packages/desktop-pet/windows/tests/task-selection.test.cjs` | 模拟 Electron、偏好与 HTTP，检查任务发现和选择规则 |
| `node --test packages/desktop-pet/windows/tests/renderer.test.cjs` | 模拟 DOM、IPC 与时钟，检查轮询保留动画、缩放取消散步及后续正常调度 |
| `node --test scripts/desktop-pet-artwork.test.mjs` | 检查默认素材复制与内容变化后的拒绝 |
| 已安装的本地包 | 在目标系统检查任务选择、WebUI 跳转、导入、缩放、隐藏恢复、启动复用，以及正常停止后保留设置 |

Windows 原生窗口检查需要 Windows x64 Electron 和 `packages/desktop-pet/windows/tests/native.cjs` 指定的环境；模拟检查不能证明原生窗口行为。完整 Host 流程和最低系统检查需要各自的实际环境。

已验证范围与限制见[支持矩阵](SUPPORT-MATRIX.md)，Windows 检查结果见[Windows 报告](WINDOWS-ADAPTATION.md)。历史桌面验证记录通过 Git 历史查询；组件检查不扩大稳定支持范围。
