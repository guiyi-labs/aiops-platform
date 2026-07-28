# Database Documentation

数据库使用 PostgreSQL，迁移文件位于 `backend/migrations/`。

原则：

- Kubernetes 实时资源不全量落库。
- kubeconfig 必须由应用层加密后保存。
- 审计记录只追加，不提供普通业务删除接口。
- 时间字段统一保存为 UTC，前端按用户时区展示。
- 每次表结构变更必须同时提交 up/down 迁移和文档更新。

## Current Migrations

| Migration | Tables / changes |
|---|---|
| `000001_init_schema` | `roles`、`users`、`user_roles`、`clusters`、`cluster_credentials`、`audit_logs` |
| `000002_auth_sessions` | `refresh_tokens`，仅保存 SHA-256 摘要、客户端信息、过期与撤销时间 |
| `000003_cluster_conditions` | 为 `clusters` 增加期望启用状态，增加 Kubernetes 风格 `cluster_conditions` |
| `000004_diagnosis_records` | `diagnosis_records` 与 `diagnosis_evidence`，保存结构化规则结论和证据快照 |
| `000005_diagnosis_workflow` | 诊断负责人/更新时间、`diagnosis_activities` 状态活动、`diagnosis_feedback` 人工反馈 |
| `000006_audit_trail` | 为 `audit_logs` 增加操作者快照、HTTP 状态、来源地址、User-Agent 和查询索引 |
| `000007_diagnosis_sla_assignment` | 诊断 SLA/解决时间、逾期索引与追加式 `diagnosis_assignments` 转派历史 |
| `000008_ai_explanations` | 追加式 AI 解释、证据引用、模型/响应标识和 token 用量 |
| `000009_ai_usage_reservations` | AI 生成前的短期 token 预算预留及过期清理索引 |
| `000010_ai_explanation_feedback` | 单用户单解释的追加式质量评价、三档结论与可选备注 |
| `000011_user_auth_version` | 为用户增加单调递增的凭据版本，使密码重置可即时撤销访问令牌 |
| `000012_diagnosis_notification_outbox` | 通知启用状态、诊断事件投递 outbox、重试/租约字段与事务内事件触发器 |
| `000013_controlled_remediation` | 受控 Deployment rollout restart 计划、确认摘要、资源版本快照、幂等租约和结果 |
| `000014_controlled_operations_catalog` | 资源来源计划的可空诊断关联、Deployment/CronJob 类型化前后值以及严格 action/参数检查 |
| `000015_saved_global_search_filters` | 当前用户私有的固定全局搜索条件、大小写不敏感名称唯一索引、schema version 与用户顺序索引 |

刷新令牌采用单次轮换：成功刷新时在同一数据库事务内撤销旧记录并创建新记录。账号被停用时刷新事务回滚，不产生可用的新会话。
会话设备列表只查询本人未撤销且未过期的 `refresh_tokens`。单会话/批量撤销在事务中先用当前 Cookie 摘要确认有效当前会话，再写 `revoked_at`；令牌摘要不离开仓储层，不进入 API、审计或前端。

`users.status` 当前使用 `active` 与 `disabled`；`user_roles` 保存用户到预置角色的多对多关系。用户角色替换、状态更新和该用户全部有效刷新令牌撤销在同一事务内执行。涉及 `system_admin` 资格的事务先取得固定 advisory lock，并在目标用户行锁下确认至少保留一个活跃系统管理员。用户名唯一约束是并发创建的最终防线，冲突映射为稳定业务错误。该能力复用现有表结构，不需要新增迁移。

`users.auth_version` 默认从 1 开始，并进入每个新访问令牌。管理员密码重置在单一事务中写入新摘要、原子递增版本并撤销所有未撤销刷新令牌。迁移部署前签发、不含版本的旧访问令牌会被拒绝；现有有效刷新会话仍可轮换获得带当前版本的新令牌，除非对应用户刚执行过密码重置。

用户主动改密复用同一字段和表，不新增迁移；更新条件同时包含用户 ID 与读取到的旧 `password_hash`。受影响行数为 0 表示凭据已变化或当前密码无效，事务不会撤销新会话或覆盖密码。

`cluster_credentials.encrypted_kubeconfig` 保存 AES-256-GCM 密文，随机 nonce 与密文一同存储；`encryption_key_version` 用于后续密钥轮换。密钥只来自进程环境，不进入数据库。删除集群时凭据和 Conditions 通过外键级联删除。

注册集群凭据替换不新增表结构：事务锁定目标集群，原子更新 `cluster_credentials` 密文/密钥版本、`clusters.api_server`，清空旧 Kubernetes 版本与探测时间，并把现有 Conditions 更新为 Unknown。这里的“凭据轮换”是单集群 kubeconfig 替换；应用主加密密钥批量再加密仍是独立的后续能力。

诊断记录保存规则 ID、严重级别、资源定位、状态、结论、根因、建议和观察时间。证据按行保存为 JSONB，并通过事务与诊断记录一起提交。历史列表不联表展开证据，详情查询按 `diagnosis_id` 顺序读取证据行。诊断不保存完整 kubeconfig、Secret 或未裁剪的任意资源 YAML。

`diagnosis_activities` 对每次合法状态变化追加一行，保存操作者 ID、名称快照、前后状态、备注和时间。`diagnosis_feedback` 保存 `accurate`、`inaccurate`、`uncertain` 三类判断及备注。用户删除后外键置空，但名称快照保留；诊断或集群删除时活动与反馈级联清理。

`diagnosis_records.sla_due_at` 是按严重级别在创建时确定的不可空截止时间；迁移对历史记录使用 `observed_at` 回填。`resolved_at` 记录最近一次解决时间，重新打开时清空。`diagnosis_assignments` 对首次自动认领和显式转派追加一行，并同时保存可置空的用户外键与不可变名称快照；删除用户不影响历史解释，删除诊断时转派记录级联清理。

`audit_logs` 只追加，不提供修改或删除业务 API。操作者和集群外键使用 `ON DELETE SET NULL`，同时保存 `actor_name`、资源名称和 `details.cluster_id` 快照，保证对象删除后仍可解释。`details` 只允许方法、路由模板和数字标识等白名单字段，不保存凭据或请求体。

`ai_explanations` 保存通过结构和引用校验的完成结果。建议操作与引用使用 JSONB；Provider、模型、非敏感响应 ID和 token 用量按行保存。API Key、完整提示词、上游错误正文和推理隐藏状态不入库。用户删除后操作者外键置空但名称快照保留，诊断删除时解释级联清理。

`ai_usage_reservations` 只保存随机预留 ID、诊断 ID、预留 token 数、创建与过期时间。它不是计费流水：成功用量来自追加式 `ai_explanations`，请求完成后预留必须删除；进程异常退出留下的记录在后续预算检查中按 `expires_at` 清理。预算事务使用 PostgreSQL advisory lock，避免多个后端实例在同一时刻以相同余额通过检查。

`ai_explanation_feedback` 保存解释外键、操作者外键与名称快照、`helpful`/`partially_helpful`/`not_helpful` 结论、最多 1000 字符的备注和创建时间。`(explanation_id, actor_user_id)` 唯一约束在数据库层阻止重复投票；删除解释时评价级联删除，删除用户时外键置空但名称快照和质量样本保留。普通 API 不提供更新或删除评价。

`notification_settings` 的固定主键行控制数据库触发器是否创建事件；应用启动时会把进程配置同步到该行。`notification_deliveries` 与诊断外键级联关联，保存 allowlist JSONB、投递状态、尝试次数、下次尝试时间、短期 claim 时间和截断后的错误摘要。诊断插入、状态变化和负责人变化由同一业务事务内的触发器追加事件，保证提交后的变化不会因进程退出而丢失。Worker 使用 `FOR UPDATE SKIP LOCKED` 原子 claim 到期记录；API 列表不返回 payload，删除测试诊断或集群时 delivery 随外键级联清理。

`remediation_plans` 保存固定目录中的 `deployment.rollout_restart`、`deployment.scale`、`cronjob.suspend` 和 `cronjob.resume`。rollout 计划必须绑定 confirmed Pod 诊断；资源来源计划的 `diagnosis_id` 必须为空。所有计划绑定目标 UID/resourceVersion 和过期时间，scale 仅保存合法的前后副本整数，CronJob 操作仅保存合法的前后 suspend 布尔值，数据库检查约束拒绝 action 与参数混用。确认 token 只保存 SHA-256 摘要；执行时以 `idempotency_key` 原子 claim，短租约允许进程故障恢复，同一 key 重放只返回已有结果，其他 key 被拒绝。计划删除不提供普通业务 API，诊断或集群删除时按来源关系清理；API 列表不返回 token 摘要、幂等键或原始 patch，资源历史最多返回 50 条。

`saved_global_search_filters` 仅保存所属用户、1..40 字符名称、2..64 字符查询词、可选 Namespace、Pod/Deployment/Service/Ingress 的规范子集和 `schema_version`。`(user_id, lower(name))` 表达式唯一索引阻止同一用户的大小写变体重名；创建事务先取得按用户派生的 PostgreSQL advisory lock，再计数并插入，使 20 条上限在并发请求下仍然成立。列表、更新和删除都包含 `user_id` 谓词，用户删除时记录级联删除。表中不保存 selector、GVK、API 路径、原始对象、结果或共享关系。
