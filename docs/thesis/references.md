# References and Attribution

更新时间：2026-07-26

本项目独立实现。参考仓库用于需求、交互、Kubernetes 权限和工程方法研究，没有把其应用源码、容器镜像内容或品牌资源复制进 `aiops-platform`。

## 行业项目

| 项目 | 本地材料 | 许可证/状态 | 本项目吸收的思路 | 明确未复制的内容 |
|---|---|---|---|---|
| KubeSphere | `E:\k8s\毕设\kubesphere-master\kubesphere-master` | 项目许可证以本地 `LICENSE` 为准，基于 Apache-2.0 并含附加条件 | 有序请求链、集群上下文、Condition、客户端缓存、统一查询、测试与许可证门禁 | 源码、API 兼容层、微内核、扩展市场、多租户体系 |
| KRM | `E:\k8s\毕设\krm-main\krm-main` | MIT；本地材料主要是 README、部署说明和截图 | 多集群资源导航、跨集群视角、资源操作流程 | 应用实现、镜像内容、账号配置、品牌与截图资产 |
| Ratel | `E:\k8s\毕设\ratel-doc-master\ratel-doc-master` | Apache-2.0；项目已停止维护 | kubeconfig 接入、ServiceAccount/RBAC、资源编辑流程 | 过时认证方式、旧 Kubernetes API、文档和图片资产 |

KubeSphere 的专项分析归档于 `docs/references/kubesphere-analysis.md`，独立实现决策归档于 ADR 0001 和 ADR 0002。

## 核心技术资料

- Kubernetes Documentation: <https://kubernetes.io/docs/>
- Kubernetes API Concepts: <https://kubernetes.io/docs/reference/using-api/api-concepts/>
- Kubernetes RBAC: <https://kubernetes.io/docs/reference/access-authn-authz/rbac/>
- Kubernetes API deprecation guide: <https://kubernetes.io/docs/reference/using-api/deprecation-guide/>
- Gin Web Framework: <https://gin-gonic.com/docs/>
- GORM Documentation: <https://gorm.io/docs/>
- PostgreSQL Documentation: <https://www.postgresql.org/docs/>
- Vue 3 Documentation: <https://vuejs.org/guide/>
- Pinia Documentation: <https://pinia.vuejs.org/>
- Vite Documentation: <https://vite.dev/guide/>
- Mermaid Documentation: <https://mermaid.js.org/intro/>

## 引用原则

- 论文中介绍参考项目时明确写“参考/借鉴”，不表述为本项目功能或原创成果。
- 引用架构思想时同时说明本科毕设的范围缩减和自主实现差异。
- 依赖版本与许可证以 `docs/thesis/dependency-licenses.md` 的生成结果为准。
- 截图只使用本系统当前版本自行采集的界面，并记录采集日期和对应 Git 提交；在初始 Git 基线尚未人工确认前，不虚构提交号。
