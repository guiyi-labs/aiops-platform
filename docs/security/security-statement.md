# M102 安全声明（Security Statement）

- Date: 2026-08-12
- Status: RC 基线（GA 前需关闭的例外见「已知例外」）
- 上位策略与威胁模型：`SECURITY.md`；权限矩阵：`docs/security/permission-matrix.md`

## 1. 身份与访问

- 本地账号 + 四角色（system_admin / system_ops_admin / operations_admin / readonly 等）；会话 JWT（access 15m / refresh 168h），`/api/v1/auth/me` 返回角色与工作区范围。
- 2D 授权矩阵：集群 × Namespace 粒度（M35/M100-A）；未授权资源返回 404 而非 403/错误详情，避免信息泄漏。
- 三层控制台（平台 → 集群 → 工作区）多租户隔离（M46-M47）；`/aiops` 查询维度强制集群/命名空间授权（M100-A）。
- OIDC/MFA：就绪准入 + 本地全链路演练（14/14，fail-closed）；真实 Provider 验收需 M89 授权。Provider 不可用时新登录 fail-closed；平台不保存 OTP seed，只消费已验证声明。

## 2. 数据保护

- 集群凭据加密存储：版本化密钥环 + 离线再加密（`backend/cmd/credential-reencrypt`）；kubeconfig 与其他凭据不入库明文、不入 Git（`.env.example` 仅为模板，默认值均为 `admin123`（本地开发口令），生产必须覆盖）。
- 敏感字段静态扫描门禁（`scripts/scan-sensitive-fields.sh`）与日志/审计脱敏契约测试（M100-C）：响应与审计详情只含路由元数据与资源身份，不含凭据、token、上游错误体。
- 审计：所有高权限操作写 `audit_logs`；离线归档带 Ed25519 签名、外部信任公钥验签、篡改拒绝（M20 Phase 10）。
- 数据库：PostgreSQL 17（本地）；逻辑备份 + WAL/PITR 演练证据（M101 本地轨）。

## 3. 受控运维边界

- 确定性规则诊断为主链路；AI 仅解释已有证据，不直接执行集群变更。
- 所有高风险操作经：权限校验 → dry-run 预览 → 人工确认（confirmation token，TTL 10 分钟）→ 幂等执行（`Idempotency-Key`）→ 审计落库。
- 固定操作目录（rollout restart / scale / image update / rollback / cronjob suspend/resume）；无任意命令、Pod exec、WebShell。
- 变更前后校验目标 UID/resourceVersion，目标漂移时拒绝执行（`remediation` 服务）。

## 4. 供应链与发布

- 依赖门禁：govulncheck 可达漏洞 0、pnpm audit 生产依赖 0 已知漏洞（M100-D）；许可证 allowlist fail-closed；基础镜像 digest 漂移 fail-closed；SBOM 差异新增包默认 fail-closed。
- 发布：SHA256SUMS、cosign 签名 fail-closed、SBOM/provenance、不可变 OCI 资产（`v0.3.0-rc.4`，20 资产 prerelease）。
- 依赖漏洞处置节奏：high/critical 30 天、其余 90 天（`SECURITY.md`）；追踪例外随 Dependabot 主版本窗口复评（`golang.org/x/crypto` openpgp 等，未调用、无修复版）。

## 5. 浏览器与前端安全

- CSP/默认拒绝网络策略、internal Service、frontend-only Ingress、受限 Pod security context（Kustomize 渲染测试覆盖）。
- 前端不展示未授权数据；响应式/无障碍门禁（Playwright + axe 双视口 0 critical/serious）。

## 6. 已知例外（GA 前处置）

| 例外 | 状态 | 处置 |
|---|---|---|
| 真实 OIDC/MFA Provider 验收（M89） | Deferred（组织授权） | 授权后两次独立演练 + 脱敏归档 |
| 真实 WAL/PITR/HA 组织演练（M90） | Deferred（组织授权） | 授权后以实测 RPO/RTO 归档 |
| 真实组织 kind/Helm 生命周期两次全新环境演练（M102 Gate D） | Deferred | 授权/CI 组织环境执行 |
| 生产监控网络抓取验证 | 部署验证项 | 投产时由监控网络验证 |

- 结论：上述例外未关闭前，**版本保持 RC，不宣称 GA**；critical 安全发现 0、high 均在有期处置计划（当前依赖扫描无新增）。
