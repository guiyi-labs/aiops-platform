# m89-oidc-local-drill：本地 OIDC Provider 全链路登录演练（身份轨）

- Date: 2026-08-12
- Status: Complete
- Scope: 在本地（无外部网络）用仓库内新建的 OIDC Provider 驱动平台真实 Authorization Code + PKCE 登录全链路，验证 14 个成功/fail-closed 场景。

## Context

M89 身份轨需要真实 OIDC discovery/JWKS、issuer/audience/nonce/state 校验、组到角色映射和 MFA 证据消费。组织授权的外部 Provider 尚未到位，先以本地可重复演练建立 fail-closed 证据，避免在真实环境里头一次踩契约。平台运行时 Provider 库（`backend/internal/oidc`）与 HTTPS 接线（`cmd/server` + `internal/httpserver/oidc.go`）已完整，此次不修改平台行为，只新增仓库内驱动工具与演练脚本。真实 Provider 验收（M89 acceptance）仍依赖组织授权。

## What Changed

### 新增本地 OIDC Provider（仓库内驱动工具，非生产 Provider）
- `backend/cmd/oidc-provider/main.go`：最小 OIDC Provider 可执行程序，运行于 HTTPS（自签证书）。端点：discovery `/.well-known/openid-configuration`、`/jwks`（RS256，kid `current`）、`/authorize`（签发一次性 authorization code，绑定 PKCE S256 challenge/state/nonce）、`/token`（校验 code_verifier 后签发 ID token：sub/iss/aud/nonce/exp/acr/`preferred_username`/`name`/`groups`）、`/logout`、`/healthz`。支持命令行参数 `-issuer/-listen/-client-id/-redirect-uri/-cert/-key/-cert-out/-key-out` 与 `?fail=` 注入模式。
- `backend/cmd/oidc-provider/main_test.go`：进程内测试覆盖 happy path、discovery/JWKS 契约、authorization code 单次使用与 PKCE S256 强制、9 组 fail 模式（wrong_nonce/omit_nonce/wrong_issuer/wrong_aud/no_mfa/low_mfa/expired/wrong_key/unsigned）。

### 新增全链路演练脚本
- `scripts/oidc-login-drill.sh`：启动 IdP + 第二个平台后端子进程（OIDC 使能，读同一本地 PostgreSQL），admin API 预关联用户，curl 驱动完整登录与 14 个场景，输出机器可读报告到 `.artifacts/oidc-drill/report-<run>.json`。因为 macOS Go 忽略 `SSL_CERT_FILE`，脚本用 `security add-trusted-cert` 临时把 IdP 证书信任进 login keychain 并在退出时删除（被 SIGKILL 时按脚本内注释手动清理）。用户/预关联幂等，可重复运行。

## Verification

- `go test -count=1 ./cmd/oidc-provider/ ./internal/oidc/`：通过。
- `go test -count=1 ./...`：后端全量通过。
- `scripts/scan-sensitive-fields.sh`：clean（1185 tracked files）。
- `scripts/oidc-login-drill.sh` 连续运行三轮收敛到 14/14 pass；最后一轮 `report-20260812-205335-3c554f.json` 全绿：
  - S1 happy path：login 302 → provider /authorize → callback 200 拿 access_token → `/me` 200 且 `operations_admin` 来自本地 user_roles。
  - S2 缺预关联：403 `OIDC_SUBJECT_NOT_PRELINKED`。
  - S3/S4 nonce 篡改与 state 篡改：502 `OIDC_LOGIN_FAILED`。
  - S5/S6 缺 MFA 证据、acr 不被接受：502 `OIDC_LOGIN_FAILED`。
  - S7 组未映射任何角色：502 `OIDC_LOGIN_FAILED`。
  - S8 轮换密钥（kid 不在 JWKS）：502 `OIDC_LOGIN_FAILED`。
  - S9 过期 token / S10 unsigned token：502 `OIDC_LOGIN_FAILED`。
  - S11 审计：`auth.oidc.callback` success 200 落库。
  - S12 Provider 运行中 token 端点不可达：login 仍 302（缓存 discovery），callback 502。
  - S13 Provider 下线后 discovery 缓存过期：login 502 `OIDC_UNAVAILABLE`。
  - S14 Provider 启动期不可达：server 以非零退出并打 `initialize OIDC provider` fatal。
- 证据：`.artifacts/oidc-drill/report-*.json`、`.artifacts/oidc-drill/tmp-*/summary.txt`、`backend.log`/`idp.log`（已 gitignore，留在本地）。运行结束时 keychain 无残留测试证书。

## Risks / Notes

- 演练 IdP 是开发/演练工具，绝不用于生产；真实 OIDC/MFA 验收需要组织批准的 Provider，状态保持 Deferred 直至授权。
- macOS 依赖 `security`（Keychain）信任自签证书；其他平台可改回 `SSL_CERT_FILE`/`SSL_CERT_DIR`（脚本头注释说明）。
- 演练脚本在宿主机另起后端进程，不干扰正在运行的 compose 栈（k8s-aiops-backend-1 等）；OIDC 用户 `oidc-operator` 与预关联会残留在本地点名库，便于重跑，重跑幂等。
- 若演练被 SIGKILL 中断，用 `security delete-certificate -c aiops-drill-oidc-provider` 清理残留信任。
