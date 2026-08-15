# Thesis Materials

此目录用于归档论文和答辩可复现材料：

- [系统架构、用例、ER 和诊断时序图](system-diagrams.md)。
- [测试矩阵与需求追踪](test-matrix.md)。
- [测试环境、软件版本和硬件配置](environment.md)。
- [10 分钟答辩演示脚本](defense-demo-script.md)。
- [答辩演示环境准备与清理](demo-environment.md)。
- [系统答辩截图与采集说明](screenshots/README.md)。
- [实验摘要（诊断覆盖 / P95 延迟 / 幂等性 / 资源成本）](experiment-summary.md)。
- [依赖许可证清单](dependency-licenses.md)，由 `scripts/generate-license-report.ps1` 生成。
- [参考项目与资料归属说明](references.md)。

故障场景 YAML 位于 `deploy/demo-scenarios`；真实实验结果和采集日期归档在 `docs/changes`，机器生成的脱敏验收证据位于已忽略的 `.artifacts`。

不要在此目录保存真实 kubeconfig、令牌、用户数据或未经脱敏的日志。
