# M110 RC-6 发布预检清单

> 状态：**本地预检全部通过，可触发 Release**（2026-08-13）。实际发布（push `v0.3.0-rc.*` tag）由用户授权后执行。

## 预检结果

| 项 | 检查 | 结果 |
|----|------|------|
| 后端编译 | `go build ./...` | ✅ exit 0 |
| 后端测试 | `go test ./... -short` | ✅ 全绿 |
| 前端 typecheck | `pnpm typecheck` | ✅ |
| 前端 lint | `pnpm lint` | ✅ |
| 前端 build | `pnpm build` | ✅（12.74s） |
| Release 工具 | `node --test scripts/release-manifest.test.mjs` | ✅ 6/6 |
| Dockerfile | `backend/Dockerfile`(51L) / `frontend/Dockerfile`(18L) | ✅ 存在 |
| Helm Chart | `deploy/helm/aiops-platform/Chart.yaml`（打包时 `--version` 覆盖） | ✅ |
| Kustomize | `deploy/kubernetes/kustomization.yaml` | ✅ |
| 迁移 | `backend/migrations/` 92 个 SQL，至 `000046`，`go:embed *.up.sql` | ✅ 自包含 |
| 离线包自包含 | drill 拷贝 init schema + 后端 embed 全迁移 | ✅ |
| Release workflow | `.github/workflows/release.yml`：validate → quality(复用 ci.yml) → package | ✅ |
| 质量门自动继承 | ci.yml 含 M109 全部门禁（覆盖率 65%、fuzz seed 含 incident/correlation、change-record job） | ✅ |
| 历史对照 | rc.4 Release run `31384939856`（14 jobs，quality gate 13 + package）success | ✅ 本流程一致且更严 |

## 触发方式

```bash
# 用户授权后，在 main 上打 RC tag 触发 Release workflow
git tag v0.3.0-rc.6
git push origin v0.3.0-rc.6
```

Release workflow 将依次：构建双架构 OCI → 生成 SBOM → helm lint/package + kustomize 校验 → 组装离线包 → provenance + release-manifest（keyless 签名）→ 上传 GitHub prerelease。

## 发布后必做（M110 验收）

1. 全新环境双路径安装演练（`scripts/offline-install-drill.sh`，`APP_VERSION=v0.3.0-rc.6`）。
2. 跨 digest 升级/回滚/备份恢复演练（`scripts/dual-env-compose-drill.sh`）。
3. `scripts/release-verify.ps1 -Version v0.3.0-rc.6` 核对资产 digest 与签名。
4. RC-6 资产 digest 固定、签名 fail-closed 确认后，M110 封口并打 `baseline-m110-rc6-YYYYMMDD`。

## 依赖外部环境

- GitHub Actions Release run（Docker Hub 可达时从 CI 构建；不可达时沿用离线重建路径）。
- Cosign keyless 签名（workflow 自带 `sigstore/cosign-installer`）。
- GitHub Release 上传（需 `contents: write` 权限，workflow 已声明）。
