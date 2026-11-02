# gofmt-drift：补齐三个未格式化文件的 gofmt

- Date: 2026-11-02
- Status: Complete
- Scope: 三个 Go 文件的纯空白对齐（`gofmt -w`），无逻辑改动

## Context

CI 的 `Backend` 作业在 `Check formatting` 一步失败，`gofmt -l .` 列出三个文件：

```
cmd/server/case_reader_adapter.go
internal/httpserver/router_harness_test.go
internal/kubernetes/service.go
```

这三处都来自 2026-10-26 之后累积、期间未经过 CI 的提交（`Backend` 的格式化检查是本仓库
最靠前的一道门，失败后 `Vet` / `golangci-lint` / `Test` 等步骤都不会执行）。与 11-02 的
前端改动无关。

## What Changed

`gofmt -w` 处理三个文件，差异合计 **6 行增 / 6 行删**，全部是**对齐空白**：

| 文件 | 改动性质 |
|---|---|
| `cmd/server/case_reader_adapter.go` | 编译期断言 var 块的等号对齐 |
| `internal/httpserver/router_harness_test.go` | 同上 |
| `internal/kubernetes/service.go` | `customResourceWhitelist` map 字面量的值对齐 |

没有任何 token、字符串、逻辑或注释内容的改变。

## Verification

- `gofmt -l .`：输出为空（全仓合规）。
- `go build ./...`：exit 0（go1.26.5 darwin/arm64）。
- `git diff --stat`：3 files changed, 6 insertions(+), 6 deletions(-)。

## 备注

格式化检查放在流水线最前，失败后后续步骤全部不执行——这意味着本次失败掩盖了
`Vet` / `golangci-lint` / 覆盖率门禁 / `OpenAPI breaking-change check` 的真实状态。
推送后需确认这些步骤本身是否也能通过。
