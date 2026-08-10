# Release Keyless Identity Ref Fix

- Date: 2026-08-11
- Status: Complete
- Scope: 修复 Release 工作流在 `workflow_dispatch` 演练下 Cosign keyless 身份校验失败的问题，使发布清单记录与实际签名身份一致。

## Context

Release 演练 run `31400327923`（package-only rehearsal，`v0.3.0-rc.5`）在
“Enforce complete signed RC” 步骤失败：

```
Cosign verification failed: none of the expected identities matched what was
in the certificate, got subjects
[https://github.com/guiyi-labs/aiops-platform/.github/workflows/release.yml@refs/heads/main]
```

根因：`scripts/release-manifest.mjs` 的 `signatureConfiguration` 把 keyless
身份硬编码为 `refs/tags/<version>`，而 `workflow_dispatch` 演练由分支
`main` 触发，Cosign 证书 subject 实际为 `refs/heads/main`。tag push 时
`$GITHUB_REF` 与硬编码一致，因此正式发布不受影响；演练路径则因身份不匹配
在最终严格校验时失败。

## What Changed

- `scripts/release-manifest.mjs`：`signatureConfiguration` 新增可选
  `identityRef` 参数；keyless 身份优先使用传入的实际 ref，缺省回退为
  `refs/tags/<version>`，保持 tag 发布行为不变。CLI `create` 子命令新增
  `--identity-ref` 选项。
- `.github/workflows/release.yml`：`release-manifest.mjs create` 现在传入
  `--identity-ref "$GITHUB_REF"`，使清单记录的身份与同一 job 中
  “Sign and verify immutable checksum root” 的签名身份一致；`verify
  --require-signatures` 从清单读取身份，自动保持同步。
- `scripts/release-manifest.test.mjs`：新增两个契约测试，分别锁定缺省
  tag 身份与显式分支 ref 身份的 `certificateIdentity`。
- `backend/internal/deployment/ci_workflows_test.go`：Release 工作流契约
  新增 `--identity-ref "$GITHUB_REF"` 必需标记，防止回归。
- `docs/release-candidate-operations.md`、`docs/ci-release.md`：把“exact tag
  identity”改为实际触发 ref 身份（tag push 用 tag ref，手动演练用分支 ref）。

## Verification

- `node --test scripts/release-manifest.test.mjs`：6/6 通过，含新增身份契约测试。
- `go test -p=1 -count=1 ./internal/deployment/`（backend 模块）：通过，
  工作流契约与发布资产契约均绿。
- `git diff --check`：无空白错误。
- 远端 Release package-only rehearsal 复跑：`31410736720`
  （https://github.com/guiyi-labs/aiops-platform/actions/runs/31410736720）
  全部成功：Required quality gate 全绿，“Build and verify RC package” 21 分 32 秒
  通过，“Sign and verify immutable checksum root” 与最终
  “Enforce complete signed RC”（此前失败的步骤）均通过，产物
  `aiops-platform-v0.3.0-rc.6` 已上传。workflow_dispatch 正确跳过
  “Publish immutable prerelease”，未创建 tag 或 Release。

## Risks / Notes

- 正式 tag 发布路径身份不变（`refs/tags/vX.Y.Z-rc.N`），行为无回归。
- 演练包的身份是分支 ref，不能作为 tag 身份的替代证据；`docs/ci-release.md`
  已说明该边界。
