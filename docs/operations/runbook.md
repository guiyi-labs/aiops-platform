# M102 运维手册（Operations Runbook）

- Date: 2026-08-12
- Status: RC 基线；适用部署形态见 [兼容范围](../testing/compatibility.md)

## 1. 快速开始（开发/演示环境）

```bash
# 1) 本地测试栈（compose，默认 15432/8080/18080）
docker compose up -d
curl -s http://127.0.0.1:8080/api/v1/health/ready   # {"status":"ready","version":...}
# 打开 http://localhost:18080，使用 BOOTSTRAP_ADMIN_USERNAME/PASSWORD 登录

# 2) 可复现闭环演示（隔离离线环境 21432/21080/21081，含 mock k8s）
./scripts/demo-drill.sh                             # 17/17 PASS（含回放场景）

# 3) 停止并清理
docker compose down -v
```

## 1.1 离线安装（空气隔离环境）

```bash
# 在可联网/镜像就绪环境组装离线包（演练布局与 M97 release 离线包对齐）
./scripts/offline-install-drill.sh          # 9/9 PASS，产出 bundle + SHA256SUMS
# 将 bundle 目录整体拷贝/传输到目标机后：
cd aiops-platform-offline-<version>
shasum -a 256 -c OFFLINE-SHA256SUMS         # 完整性校验必须全部 OK
for f in images/*.tar; do docker load -i "$f"; done
docker compose -f deploy/compose.offline.yaml up -d   # pull_policy: never，无需网络
# 复验：/api/v1/health/ready、登录、/me → system_admin
```

## 2. 健康与就绪

| 端点 | 用途 |
|---|---|
| `GET /api/v1/health/live` | 存活（无需鉴权） |
| `GET /api/v1/health/ready` | 就绪（数据库连通、迁移完成；返回 `status` 与 `version`） |

compose 已内置 healthcheck（postgres `pg_isready`、backend ready 探测、frontend HTTP 200）；`docker compose ps` 应全部 `healthy`。

## 3. 升级与回滚

同一不可变镜像集内：

```bash
# 升级：将 compose 的 backend/frontend 镜像引用切到新 digest 后
docker compose up -d --force-recreate backend frontend
# 验证：/api/v1/health/ready 的 version 已变更、关键旅程可登录、数据标记保持

# 回滚：切回原 digest 后再次 --force-recreate 并复验
```

- 跨 digest 一致性证据：`scripts/dual-env-compose-drill.sh`（`APP_UPGRADE_BACKEND_IMAGE`，14/14；`APP_BACKUP_RESTORE=1` 16/16）；最近一轮以 `v0.3.0-rc.4` 基线 + `v0.3.0-rc.5-replay` 升级目标复跑，install/upgrade/rollback/restore 全 PASS（`report-20260812-225042-e68b90.json`）。
- 升级前必须先做逻辑备份（见下节）；升级后验证审计/告警等写入路径。

## 4. 备份与恢复

- 逻辑备份（独立防线）：
  ```bash
  # 运行中实例
  docker compose exec postgres pg_dump -U aiops -d aiops > backup-$(date +%Y%m%d-%H%M%S).sql
  # 全新空库还原：新 postgres 卷（不挂 initdb 预种子）后
  docker compose exec -T postgres psql -U aiops -d aiops < backup.sql
  # 复验：登录 + /api/v1/auth/me → 角色正确；业务标记/计数一致
  ```
  演练证据：`APP_BACKUP_RESTORE=1 ./scripts/dual-env-compose-drill.sh`（16/16）。
- WAL/PITR（生产韧性，本地证据）：
  `./scripts/wal-pitr-drill.sh`（8 场景，含 PITR、缺 WAL 快速失败、崩溃注入、备库切换、网络分区、归档目标故障）。真实基础设施演练需 M90 授权，RPO/RTO 以实测为准。

## 5. 关键旅程验收（每次发布前）

1. 登录 → `/api/v1/auth/me` → `system_admin`。
2. 态势：`GET /api/v1/fleet/health`、集群资源列表。
3. 根因与证据：`POST /api/v1/clusters/:id/diagnoses`（Node/Pod）→ `GET /api/v1/diagnoses/:id`（evidence/root_causes/recommendations）。
4. 受控动作：诊断 `PATCH` → `confirmed` → `POST /diagnoses/:id/remediations/preview` → 携带 `Idempotency-Key` `POST /remediations/:id/execute` → `succeeded`；随后在集群侧验证变更已落地。
5. 事故复盘：`POST /api/v1/incidents` → note → resolve → postmortem → `GET /incidents/:id/export`（CSV）。
- 自动化验收：`./scripts/demo-drill.sh` 全链路 17 项断言（含 M94 回放动作前/后各一组）。

## 6. 日志、审计与安全运维

- 后端日志：`docker compose logs -f backend`（zap）。
- 审计：`audit_logs` 表持久化；离线归档与签名验签见 `backend/cmd/audit-archive`、`scripts/e2e-audit-archive.ps1`、`docs/security/`。
- 敏感信息扫描：`./scripts/scan-sensitive-fields.sh`（提交前必须 clean）。
- 依赖/供应链门禁：`./scripts/dependency-vuln-scan.sh`、`./scripts/license-scan.sh`、`./scripts/image-base-drift.sh`、`./scripts/sbom-diff.mjs`。
- 凭据轮换：`backend/cmd/credential-reencrypt`（版本化密钥环）；集群 kubeconfig 更新 `PATCH /api/v1/clusters/:id` 后重探活。

## 7. 故障排查

| 症状 | 检查 |
|---|---|
| backend 未就绪 | `docker compose logs backend`；确认 postgres healthy、迁移成功、`DATABASE_URL` 正确 |
| 登录失败 | `BOOTSTRAP_ADMIN_*` 初始化仅对全新库生效；确认 `.env` 与运行中容器一致 |
| 集群 probe 失败 | kubeconfig 必须为 HTTPS、含 token 或客户端证书；`PATCH /clusters/:id` 更新凭据后 `POST /probe` |
| 受控动作执行失败 | 诊断需 `confirmed` 且资源匹配；`Idempotency-Key` 8–128 字符；plan TTL 10 分钟 |
| OOM 构建失败 | colima 2GiB 限制：宿主机 `GOOS=linux GOARCH=arm64 go build` 后小镜像封包 |

## 8. 演练/测试环境清理不变量

- 所有本地演练（dual-env、demo-drill、oidc-login）结束后必须 `docker compose -f <file> down -v` 无残留容器/卷/网络。
- `.artifacts/` 为本地证据目录（gitignore），不随仓库分发；CI/组织环境复验需重新生成。
