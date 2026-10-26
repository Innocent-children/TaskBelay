# 画图

## 先想清楚图要讲什么

图用来讲文字讲不清的东西，所以方案里最难用文字讲清的那部分最该画。好画的部分不一定是最需要画的部分。检验办法：遮住文字只看图，看自己能不能讲出图想表达的内容。

按方案要讲清的东西选图：

| 要讲清的 | 合适的图 |
| --- | --- |
| 一件事分几步处理、在哪里分岔 | 流程图 `flowchart` |
| 由哪些部分组成、谁调用谁 | 结构图（用 `flowchart` 画方框和箭头） |
| 几方之间按什么先后交互 | 时序图 `sequenceDiagram` |
| 一个对象有哪些状态、怎么切换 | 状态图 `stateDiagram-v2` |
| 规则和时间有关：什么时候开始、等到什么时候、到期后怎样 | 用一个具体例子按时间排开（`flowchart LR` 或 `gantt`） |
| 分几个阶段推进 | 横向排开的阶段图 |
| 改之前和改之后有什么不同 | 两张并排的小图 |

一张图讲不完时，可以先画一张交代整体的图，让读者知道改动在什么位置，再画一张展开改动的核心部分。数量由内容决定，一张够就画一张。

不管画哪种图，主要的出错情况会怎样都要能看到。

## 图上的字

- 写清这一部分做什么，不只写名字。代码里的类名有助于定位时，放在第二行。
- 关键参数直接写在图上，读者不用回正文找。
- 每个方框 1 到 3 行，用 `<br/>` 手动换行，每行十几个字以内。
- 连线上的字写条件或动作，越短越好。判断框里只写短问句，条件细节写到连线上。

## 颜色

按角色给方框着色，同一份文档里，同一个角色始终用同一个颜色。角色由方案自己定：想突出改动范围，就分"新增 / 修改 / 不变"；想突出出错处理，就分"正常 / 出错"；也可以按调用方、处理、存储、结果来分。类别最好不超过五六种，颜色太多就不再传达信息。

## 样式

下面这套样式来自用户认可的范例，中文字体和配色都调好了，可以直接用，也可以改。示例里的方框只是演示写法。

```mermaid
%%{init: {"theme": "base", "themeVariables": {"fontFamily": "PingFang SC, Microsoft YaHei, sans-serif", "fontSize": "16px", "edgeLabelBackground": "#ffffff", "lineColor": "#64748b"}, "flowchart": {"curve": "basis", "nodeSpacing": 48, "rankSpacing": 36, "wrappingWidth": 280}}}%%
flowchart TB
    A["步骤一<br/>关键参数"] --> B{"条件成立？"}
    B -->|"是"| C["步骤二"]
    B -->|"否"| D["出错处理"]
    C --> E["结果"]
    D --> E

    classDef blue fill:#eff6ff,stroke:#60a5fa,color:#1e3a8a;
    classDef gray fill:#f8fafc,stroke:#94a3b8,color:#0f172a;
    classDef orange fill:#fff7ed,stroke:#fb923c,color:#9a3412;
    classDef teal fill:#f0fdfa,stroke:#2dd4bf,color:#115e59;
    classDef green fill:#f0fdf4,stroke:#4ade80,color:#166534;
    classDef red fill:#fef2f2,stroke:#f87171,color:#991b1b;

    class A,C gray;
    class B orange;
    class D red;
    class E green;
```

上面是纵向、曲线连接的版本，适合步骤和分岔较多的图。横向、直线连接、排得更紧凑的版本，把第一行换成：

```text
%%{init: {"theme": "base", "themeVariables": {"fontFamily": "PingFang SC, Microsoft YaHei, sans-serif", "fontSize": "16px", "edgeLabelBackground": "#ffffff", "lineColor": "#64748b"}, "flowchart": {"curve": "linear", "nodeSpacing": 30, "rankSpacing": 40}}}%%
```

并把 `flowchart TB` 改成 `flowchart LR`。

六种颜色怎么分配由你决定。一般来说，橙色最显眼，留给读者最该注意的地方；绿色适合表示结果或正常状态；红色适合表示出错；灰色给普通步骤。

## Mermaid 排版技巧

- 方框 ID 用英文（`SAVE`、`CHECK`），显示的文字放在引号里，括号、斜杠这类字符就不会出错。
- 连线交叉多时，先调整方框的声明顺序；还不行就加大 `nodeSpacing` 和 `rankSpacing`。
- subgraph 会把方框拉到一起，只在它们确实是一组时才用。
- 想让两个方框并排又不连线，用 `A ~~~ B`。

## 渲染检查

本机有 mermaid-cli（`mmdc`）时，可以直接渲染整份 Markdown，每张图生成一张 PNG：

```bash
mmdc -i 方案.md -o /临时目录/方案.md -e png -s 2 -b white
```

输出放到临时目录，不要覆盖原文。然后打开 PNG 看：文字有没有溢出、连线有没有压字或乱成一团、整体是否失衡。没有 mmdc 时，用支持 Mermaid 的预览（IDE 的 Markdown 预览、Git 平台页面）看，或者告诉用户在哪里预览。
