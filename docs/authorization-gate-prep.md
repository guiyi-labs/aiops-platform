# M89/M90 授权轨准备材料（GA Gate D 前置）

- Status: Draft（本地准备材料；授权未获取前状态保持 Deferred）
- Updated: 2026-08-12
- 关联：`docs/next-long-term-plan.md` M101/M102、ADR 0052（生产 OIDC/MFA）、
  ADR 0033（恢复就绪准入）、`docs/testing/known-limitations.md`

## 1. 为什么需要授权（Gate D 唯一外部缺口）

- GA 准入（Gate D）要求 M89/M90/M100/M101 完成，其中 M100/M101 本地轨已闭环；
  剩余未关闭项全部落在两条组织授权轨：
  - **M89 身份轨**：真实 OIDC discovery/JWKS、issuer/audience/nonce/state 校验、
    组→角色映射、MFA 声明消费、Provider 不可用 fail-closed、会话策略与断供。
  - **M90 数据轨**：真实 WAL 归档/PITR、多副本 HA、故障注入（磁盘压力 ENOSPC、
    跨主机网络中断/崩溃）、以实测 RPO/RTO 验收。
- 不满足前只发布 RC，不宣称 GA；本地 mock/演练工具（`oidc-provider`、
  `demo-kube-mock`）绝不可充当生产证据。

## 2. 申请所需组织资源（最小授权包）

| 轨 | 资源 | 交付给平台团队的凭证/资料 |
|---|---|---|
| M89 身份 | 组织 IdP 租户 + 应用注册（Authorization Code + PKCE，签发 id_token） | discovery 端点、client_id、需服务端保管的 client_secret、JWKS 端点、测试账号与组、MFA 验证账号 |
| M89 身份 | 授权放行（允许把 OIDC 设为 `OIDC_ENABLED=true`） | 变更窗口与回滚负责人 |
| M90 数据 | 生产级 PostgreSQL 基础设施（主从/多副本、独立归档存储） | 连接方式、归档桶/路径、PITR 窗口、RPO/RTO 目标批准件 |
| M90 数据 | 演练窗口与故障注入许可（ENOSPC、网络分区、SIGKILL、恢复点验证） | 演练计划审批 + 演练后数据完整性核对人 |

## 3. 验收范围（拿到资源后逐项验证）

### M89 身份轨验收
- [ ] discovery/JWKS 拉取与缓存、issuer/audience/nonce/state 全部校验通过；
- [ ] 组→角色映射按固定 allowlist，未知组不授任何角色；
- [ ] 特权角色要求 MFA 证据声明，缺失即拒绝（fail-closed）；
- [ ] Provider 不可达/密钥轮换/过期 token 全部 fail-closed（新登录拒绝，既有会话按策略失效）；
- [ ] `external_identities` 预关联表生效（未预关联用户 403）；
- [ ] 断供场景下保留的本地 break-glass 账号可登录并触发高优先级审计/通知；
- [ ] 审计落库：登录/失败/映射/断供事件可追溯。

### M90 数据轨验收
- [ ] WAL 归档持续可用，gap 监控无漏窗；归档目标故障时积压可排空；
- [ ] PITR 恢复到指定时间点且数据一致（无损行数校验）；
- [ ] 迁移前逻辑备份可作为独立防线；
- [ ] 多副本优雅停机/故障切换/网络分区重连演练通过；
- [ ] ENOSPC 故障注入下主库行为符合预期（不丢已提交数据，恢复后可写）；
- [ ] RPO/RTO 只引用本次实测值（不预写达成），报告脱敏归档。

## 4. 本地已有证据（申请时可附）

- `scripts/oidc-login-drill.sh`：14/14 场景（真实 Authorization Code + PKCE、acr MFA、
  9 种失败注入 fail-closed、审计落库），报告 `.artifacts/oidc-drill/`；
  记录 `docs/changes/2026-08-12-m89-oidc-local-drill.md`。
- `scripts/wal-pitr-drill.sh`：8 场景（无损 PITR、时间点恢复、缺 WAL 快速失败、
  迁移前逻辑备份、SIGKILL 崩溃注入、流式备库/故障切换、网络分区、归档目标故障），
  实测 RPO≤2s、RTO≈1.2–2.7s（本地观测值）；记录
  `docs/changes/2026-08-12-m101-wal-pitr-drill.md`。
- 平台侧：`backend/internal/oidc` Provider 库 + HTTPS 接线已完整；恢复就绪准入
  （ADR 0033）15 项控制；双环境安装/升级/回滚/备份恢复全 PASS
  （`docs/changes/2026-08-12-m102-rc5-replay-dual-env-evidence.md`）。

## 5. 授权后的执行顺序（草案）

1. M89：配置组织 IdP（`OIDC_ENABLED=true` + discovery/client/secret）→ 预关联表导入
   → 组角色映射核对 → 按第 3 节逐项验收 → 断供演练 → 脱敏证据归档。
2. M90：按批准的拓扑部署多副本 + 归档 → 跑 `wal-pitr-drill` 场景子集（磁盘压力、
   跨主机网络分区、故障切换）→ 记录实测 RPO/RTO → 两次独立全新环境演练。
3. 汇入 Gate D：M89/M90 证据 + M100/M101 本地轨 → 两次独立演练一致 → 更新
   `docs/testing/known-limitations.md` 逐项关闭 → 版本声明 GA 或保持 RC+缺口说明。

## 6. 当前状态与动作

- 状态：Deferred（未授权）。已完成的本地准备工作：平台接线、本地演练工具、验收清单、
  证据引用与执行顺序（本文档）。
- 待办（需人）：向组织提出第 2 节最小授权包；授权后按第 5 节执行。
