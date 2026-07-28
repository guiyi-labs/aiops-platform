# System Diagrams

更新时间：2026-07-26

本页图表使用 Mermaid 保存为文本源，论文定稿时可统一导出为 SVG/PNG。图中只描述本项目自主实现的边界，不把参考项目组件作为系统组成部分。

## 系统架构图

```mermaid
flowchart LR
    User["平台用户"] -->|HTTPS / JSON| Web["Vue 3 Web Console"]
    Web -->|/api/v1| API["Go API Service"]
    Monitor["监控采集器"] -->|GET /metrics| API

    subgraph Backend["模块化单体后端"]
        API --> Pipeline["请求链路\nRequest ID / Auth / RBAC / Audit / Metrics"]
        Pipeline --> Registry["集群注册与凭据管理"]
        Pipeline --> Gateway["只读 Kubernetes Gateway"]
        Pipeline --> Diagnosis["确定性诊断引擎"]
        Pipeline --> Remediation["受控处置服务"]
        Pipeline --> AI["AI 解释服务"]
        Pipeline --> Notification["通知 Outbox Worker"]
    end

    Registry --> DB[("PostgreSQL 17")]
    Diagnosis --> DB
    Remediation --> DB
    AI --> DB
    Notification --> DB
    Registry --> K8sA["Managed Kubernetes A"]
    Gateway --> K8sA
    Gateway --> K8sB["Managed Kubernetes B / kind"]
    Remediation -->|"固定 dry-run + patch"| K8sB
    AI -->|"store=false / JSON Schema"| Provider["OpenAI-compatible Provider"]
    Notification -->|"HMAC-SHA256 Webhook"| Receiver["External Receiver"]
```

## 角色与核心用例

```mermaid
flowchart TB
    Admin["系统管理员"] --> UserAdmin["用户与角色管理"]
    Admin --> ClusterAdmin["集群接入、启停、凭据轮换"]
    Ops["运维管理员"] --> ResourceRead["跨集群资源查询"]
    Ops --> Diagnose["创建诊断、确认、转派"]
    Ops --> Remediate["预演并确认受控处置"]
    Auditor["安全审计员"] --> Audit["审计查询与 CSV 导出"]
    Viewer["只读用户"] --> ResourceRead
    Viewer --> History["诊断与 AI 解释历史"]
    Diagnose --> Evidence["规则结论与证据快照"]
    Evidence --> History
    Evidence --> Explain["按需 AI 解释与评价"]
```

## 核心数据 ER 图

```mermaid
erDiagram
    USERS }o--o{ ROLES : assigned_by_USER_ROLES
    USERS ||--o{ REFRESH_TOKENS : owns
    USERS ||--o{ AUDIT_LOGS : acts
    CLUSTERS ||--|| CLUSTER_CREDENTIALS : encrypts
    CLUSTERS ||--o{ CLUSTER_CONDITIONS : reports
    CLUSTERS ||--o{ DIAGNOSIS_RECORDS : scopes
    DIAGNOSIS_RECORDS ||--o{ DIAGNOSIS_EVIDENCE : contains
    DIAGNOSIS_RECORDS ||--o{ DIAGNOSIS_ACTIVITIES : tracks
    DIAGNOSIS_RECORDS ||--o{ DIAGNOSIS_ASSIGNMENTS : assigns
    DIAGNOSIS_RECORDS ||--o{ DIAGNOSIS_FEEDBACK : evaluates
    DIAGNOSIS_RECORDS ||--o{ AI_EXPLANATIONS : explains
    AI_EXPLANATIONS ||--o{ AI_EXPLANATION_FEEDBACK : evaluates
    DIAGNOSIS_RECORDS ||--o{ NOTIFICATION_DELIVERIES : publishes
    DIAGNOSIS_RECORDS ||--o{ REMEDIATION_PLANS : controls

    USERS {
        bigint id PK
        string username UK
        string password_hash
        int auth_version
        string status
    }
    CLUSTERS {
        bigint id PK
        string name UK
        string api_server
        bool enabled
    }
    CLUSTER_CREDENTIALS {
        bigint cluster_id PK_FK
        bytes ciphertext
        string key_version
    }
    DIAGNOSIS_RECORDS {
        bigint id PK
        bigint cluster_id FK
        string rule_id
        string resource_uid
        string status
        timestamp due_at
    }
    DIAGNOSIS_EVIDENCE {
        bigint id PK
        bigint diagnosis_id FK
        string evidence_type
        jsonb payload
    }
    AI_EXPLANATIONS {
        bigint id PK
        bigint diagnosis_id FK
        string model
        jsonb citations
        int input_tokens
        int output_tokens
    }
    REMEDIATION_PLANS {
        uuid id PK
        bigint diagnosis_id FK
        string action
        string target_uid
        string status
        timestamp expires_at
    }
```

## 诊断与受控处置时序图

```mermaid
sequenceDiagram
    actor Operator as 运维管理员
    participant Web as Vue Console
    participant API as Go API
    participant K8s as Kubernetes API
    participant Rules as Diagnosis Engine
    participant DB as PostgreSQL

    Operator->>Web: 选择异常 Pod 并发起诊断
    Web->>API: POST /clusters/{id}/diagnoses
    API->>K8s: GET Pod / Event / owner Deployment
    K8s-->>API: 当前状态与事件
    API->>Rules: 规则匹配和证据构造
    Rules-->>API: rule_id + severity + evidence
    API->>DB: 事务保存诊断、证据、通知 outbox
    DB-->>API: diagnosis_id
    API-->>Web: 可追溯诊断结果

    Operator->>Web: 确认诊断并请求 rollout restart
    Web->>API: POST remediations/preview
    API->>K8s: PATCH Deployment (dryRun=All)
    K8s-->>API: dry-run accepted
    API->>DB: 保存限时计划与 token hash
    API-->>Web: plan + one-time confirmation token
    Operator->>Web: 二次确认
    Web->>API: POST /remediations/{id}/execute + Idempotency-Key
    API->>DB: 锁定计划并校验 token / TTL / resourceVersion
    API->>K8s: 固定 annotation patch
    K8s-->>API: Deployment updated
    API->>DB: 标记 succeeded 并追加审计
    API-->>Web: 同一计划结果
```

## 安全边界说明

- 平台数据库不镜像保存 Kubernetes 实时对象，只保存平台元数据、诊断快照和审计记录。
- kubeconfig 使用 AES-256-GCM 加密，接口、日志、论文材料和验收证据均不得包含明文凭据。
- Kubernetes Gateway 是显式资源白名单，不提供任意 URL 代理。
- 受控处置只实现 `deployment.rollout_restart`，必须经过 confirmed 诊断、服务端 dry-run、一次性 token、resourceVersion 和幂等键校验。
- AI 位于确定性规则之后；失败、超时或预算不足不会改变诊断主结果。
