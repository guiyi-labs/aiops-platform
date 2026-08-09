# M88: 发布闭环本地化 + 签名门禁 fail-closed

- Date: 2026-08-09
- Status: Development Complete（本地验证通过；真实 GitHub Release / cosign 仍依赖组织授权）
- Scope: 供应链交付本地可执行部分

## Context

M88 正式发布闭环的托管 CI 路径依赖 GitHub token 与组织授权（创建 Release、
Fulcio keyless OIDC 签名），本轮不伪造外部证据，而是补齐本地可验证部分：
离线发布包组装 + SHA256 校验 + 签名状态门禁，并把 CI 的 cosign
provenance 步骤从 fail-open 改为 fail-closed，保证未签名产物不可发布。

## What Changed

- 新增 scripts/release-verify.ps1（M88 本地发布校验）：
  - 校验语义化版本格式 vX.Y.Z（失败即退出）；
  - 以 HEAD 生成 source tar.gz、OpenAPI 契约、许可证清单、
    Helm Chart.yaml 与 helm tar.gz；
  - 生成 release-metadata.json（version/revision/builder/arch/images/signature 状态）
    与 SHA256SUMS，并逐文件自校验 SHA-256 一致；
  - cosign 存在时用 COSIGN_PRIVATE_KEY / COSIGN_PUBLIC_KEY 做真实
    sign-blob + verify-blob 循环；缺失时写入 SIGNING_SKIPPED 哨兵并 warning；
  - -IncludeImages 开关用于需要多架构 OCI tar 的主机（默认由 CI/release.yml 产出）。
  - 本地实证：SHA256SUMS verified for 7 files（revision 658066c）。
- 增强 .github/workflows/release.yml：
  - cosign attest-blob 原 fail-open 改为 fail-closed（任一签名失败立即 exit 1）；
  - Publishing 前新增签名门禁步骤，要求 SHA256SUMS.sig / .cert.pem / provenance.sig 均存在；
  - 工作流契约测试 TestCIWorkflowContractsAreParseableAndBounded 通过。

## 授权缺口（如实记录，不伪造）

- GitHub Release 创建、tag 推送、Fulcio keyless 签名需要组织授权与 runner 身份；
- 本地无 cosign/syft，签名路径保持 有工具才有签名、无工具明确 skip。

## Acceptance

- scripts/release-verify.ps1 -Version v0.2.0 本地通过（SHA256SUMS 7 文件全绿）；
- 工作流测试通过；release.yml fail-closed 门禁已生效。
