# 企微告警限流改造方案

## 背景

在企微告警多的时候（如压测期间），多个入口同时向同一个企微机器人发送大量告警，超过企微规定的每分钟 20 条限制，导致部分通知被拒绝。

## 目标

- **不超频**：任意连续 60 秒内，向同一个机器人发出的请求不超过 18 次，比企微上限少 2 次作为余量。
- **不丢告警**：超出额度的告警先暂存，稍后补发，不丢弃。
- **少发消息**：同一类告警合并成一条汇总消息发送，用更少的请求送达更多告警，也减少群里的刷屏。
- **平时无感知**：告警量正常时，发送方式和现在完全一样，即时、逐条送达。

## 思路

本次采用[精确滑动窗口算法（Sliding Window Log）](https://redis.io/tutorials/howtos/ratelimiting/)，用 Redis 记录每个机器人的请求时间，将任意连续 60 秒内的 HTTP 请求数控制在 18 次，为企微的上限留出余量。告警先保存到 Redis，没有积压且次数充足时逐条发送；需要等待时按模板合并，由延迟队列在最早可发送时间唤醒后台消费者，逐组补发。积压全部发完后，新告警恢复逐条处理。

## 流程解析

现有所有企微告警都会汇集到一个出口，所以，只改动这个出口即可。

```mermaid
%%{init: {"theme": "base", "themeVariables": {"fontFamily": "PingFang SC, Microsoft YaHei, sans-serif", "fontSize": "16px"}, "flowchart": {"curve": "linear", "nodeSpacing": 30, "rankSpacing": 40}}}%%
flowchart LR
    subgraph ENTRIES["现有告警入口"]
        HTTP["业务方调用 HTTP 接口发送即时告警<br/>v1/message/sendWechatInstantAlarm"]
        HEAP["定时检查消息堆积情况并发送告警<br/>MessageHeapUpHandler"]
        ABNORMAL["定时统计异常消息数量并发送告警<br/>AbnormalMessageNumHandler"]
        OTHER_SERVICES["其他服务通过 MQ 发送告警消息"]
        MQ["接收告警 Topic 中的消息<br/>yansi_yx_message_alarm<br/>MessageAlarmWeChatListener"]
        MONITOR["定时检查 MQ 消费组积压情况并发送告警<br/>MQMonitorHandler"]

        HEAP -->|"发送 MQ 消息"| MQ
        ABNORMAL -->|"发送 MQ 消息"| MQ
        OTHER_SERVICES -->|"发送 MQ 消息"| MQ
    end

    HTTP --> DISPATCH
    MQ --> DISPATCH
    MONITOR --> DISPATCH
    DISPATCH["现有企微发送方法"] --> SAVE[("Redis 保存待发送告警")]
    SAVE --> LIMIT["按机器人限流<br/>立即发送或等待合并补发<br/>详细过程见下图"]
    LIMIT --> SEND["调用企微<br/>更新发送结果"]
    SEND --> WECOM["企微群收到通知"]

    classDef entry fill:#eff6ff,stroke:#60a5fa,color:#1e3a8a;
    classDef process fill:#f8fafc,stroke:#94a3b8,color:#0f172a;
    classDef quota fill:#fff7ed,stroke:#fb923c,color:#9a3412;
    classDef done fill:#f0fdf4,stroke:#4ade80,color:#166534;
    classDef stored fill:#f0fdfa,stroke:#2dd4bf,color:#115e59;

    class HTTP,HEAP,ABNORMAL,OTHER_SERVICES,MQ,MONITOR entry;
    class DISPATCH,SEND process;
    class LIMIT quota;
    class SAVE stored;
    class WECOM done;
```

具体细节：

```mermaid
%%{init: {"theme": "base", "themeVariables": {"fontFamily": "PingFang SC, Microsoft YaHei, sans-serif", "fontSize": "16px", "edgeLabelBackground": "#ffffff", "lineColor": "#64748b"}, "flowchart": {"curve": "basis", "nodeSpacing": 48, "rankSpacing": 36, "wrappingWidth": 280}}}%%
flowchart TB
    NORMAL["新告警逐条发送<br/>发送前检查次数"]
    MERGE["Redis 合并待发送告警<br/>积压期间，新告警持续加入"]
    WAIT["Redisson 延迟队列<br/>按最早可发送时间安排通知"]

    NORMAL -->|"额度不足"| MERGE
    MERGE --> WAIT
    WAIT -->|"到时唤醒"| PICK["补发处理线程<br/>按等待顺序取一组<br/>生成一条汇总消息"]
    PICK --> QUOTA{"最近60秒请求数少于18次？"}
    QUOTA -->|"是"| SEND["记录本次请求时间<br/>一次 HTTP 请求发送本组"]
    QUOTA -->|"否，保留本组等待"| WAIT
    SEND -->|"本组发送成功"| BACKLOG{"还有积压吗？"}
    BACKLOG -->|"有，再取下一组"| PICK
    BACKLOG -->|"无，恢复逐条发送"| NORMAL

    classDef normal fill:#f0fdf4,stroke:#86efac,color:#166534;
    classDef process fill:#f8fafc,stroke:#94a3b8,color:#0f172a;
    classDef quota fill:#fff7ed,stroke:#fdba74,color:#9a3412;
    classDef sending fill:#eff6ff,stroke:#60a5fa,color:#1e3a8a;

    class NORMAL normal;
    class MERGE,WAIT,PICK,BACKLOG process;
    class QUOTA quota;
    class SEND sending;
```

1. **额度充足时，逐条发送。** 没有积压时，新告警按原有方式逐条发送。每次调用企微前，消息服务通过 Lua 脚本检查 Redis 中该机器人最近 60 秒的请求次数；少于 18 次，就记录本次请求时间并发送。

2. **合并待发送告警。** 次数用完后，后续告警保存在 Redis 中。同一机器人内，相同模板 ID 的告警合并成一组；模板 ID 为 0 时，按正文开头的固定字数合并。每组按最早一条告警的到达时间排队；一组被取出后，再来的同模板告警另起一组。

3. **安排唤醒时间。** 额度不足时，消息服务立即计算最早可发送时间：当前 18 次请求中最早一次的发送时间加 60 秒。延迟队列由 Redisson 实现，消息服务通过该队列安排延迟通知。到达预定时间后，由消息服务的补发处理线程处理该机器人的积压告警。

4. **发送当前分组。** 同一机器人的分组按等待顺序依次发送。补发处理线程将当前组的数量、时间范围和告警原文组成一条汇总消息，按企微要求控制长度。发送前检查最近 60 秒的请求次数：少于 18 次，就记录本次请求时间并通过一次 HTTP 请求发送；达到 18 次，则保留本组，按最早可发送时间重新安排唤醒。

5. **发送成功后，判断是否继续补发。** 当前组发送完成后，再检查剩余积压。有积压就取下一组；积压清空就结束补发，后续新告警恢复逐条发送。
