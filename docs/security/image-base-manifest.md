# Image Base Manifest

M100-D：镜像基础层锁定清单。所有平台镜像的基础层必须与下表一致（按 tag 解析的
manifest digest），任何基础镜像漂移必须先更新本清单并经过审查，再发布新版本。
`scripts/image-base-drift.sh` 在 CI 中强制执行。

| 用途 | Base Image | 解析时间（UTC） | Manifest Digest |
|---|---|---|---|
| backend build | `golang:1.26-alpine` | 2026-08-12 | `sha256:0178a641fbb4858c5f1b48e34bdaabe0350a330a1b1149aabd498d0699ff5fb2` |
| backend runtime | `alpine:3.22` | 2026-08-12 | `sha256:14358309a308569c32bdc37e2e0e9694be33a9d99e68afb0f5ff33cc1f695dce` |
| frontend build | `node:22.13.1-alpine3.21` | 2026-08-12 | `sha256:e2b39f7b64281324929257d0f8004fb6cb4bf0fdfb9aa8cedb235a766aec31da` |
| frontend runtime | `nginx:1.27-alpine` | 2026-08-12 | `sha256:65645c7bb6a0661892a8b03b89d0743208a18dd2f3f17a54ef4b76fb8e2f2a10` |

## 更新流程

1. 有意的基础镜像升级：更新 `backend/Dockerfile` / `frontend/Dockerfile` 的 FROM，
   解析新 tag 的 manifest digest，更新上表，并在 change-record 中说明升级理由与验证。
2. 意外漂移（tag 被上游重新指向）：`scripts/image-base-drift.sh` 会失败，必须先
   审查新 digest 并显式更新清单后才能合入。
