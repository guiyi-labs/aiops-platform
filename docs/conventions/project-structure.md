# Directory And File Conventions

状态：Accepted

## General Rules

- 目录和文件名使用小写英文；多个单词使用 kebab-case。
- Go 文件和包使用小写英文，文件名使用 snake_case。
- Vue 组件使用 PascalCase，组合函数使用 `useXxx.ts`，API 模块使用 kebab-case。
- 一个文件只承担一个清晰职责，禁止使用 `utils`、`common` 作为无边界收纳目录。
- 生产代码、测试、部署清单和文档材料分开存放。
- 自动生成文件必须标记来源，不手工修改。

## Backend

```text
backend/
  cmd/server/             application entry point
  internal/config/       environment configuration
  internal/httpserver/   routes, middleware and handlers
  internal/<domain>/     domain services and repositories
  internal/store/        database infrastructure
  migrations/            ordered SQL migrations
```

- HTTP handler 不直接调用 client-go 或数据库。
- domain service 不依赖 Gin。
- 接口错误通过统一错误码返回，不把底层错误直接暴露给前端。
- 测试文件与被测 Go 文件同目录，命名为 `*_test.go`。

## Frontend

```text
frontend/src/
  api/          typed HTTP clients
  assets/       repository-owned static assets
  components/   reusable UI components
  layouts/      page shells
  router/       routes and guards
  stores/       Pinia stores
  types/        shared TypeScript types
  views/        route-level views
```

- 页面组件只负责组合，不在页面内散落 HTTP 调用。
- API 响应必须定义 TypeScript 类型。
- 共享状态进入 Pinia；局部交互状态保留在组件内。
- 表格、工具栏和状态区域使用稳定尺寸，避免加载时布局跳动。

## Documentation

- 架构变更先新增或更新 ADR。
- 每个开发阶段在 `docs/changes/YYYY-MM-DD-<topic>.md` 记录范围、验证和遗留项。
- API、数据库迁移和演示场景发生变化时同步更新对应文档。
- 项目截图和实验数据放入 `docs/`，注明版本和采集日期。

## Migration Naming

```text
000001_init_schema.up.sql
000001_init_schema.down.sql
000002_add_cluster_status.up.sql
000002_add_cluster_status.down.sql
```

已进入共享环境的迁移不得修改，只能追加新迁移。
