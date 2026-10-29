package main

import (
	"context"

	"k8s-aiops.local/backend/internal/apiquery"
	"k8s-aiops.local/backend/internal/kubernetes"
)

// topologyNamespaceLister 把 kubernetes.Service 的 namespace 列表适配成
// topology.NamespaceLister（只关心名字）。
//
// 为什么不直接让 kubernetes.Service 实现那个接口：它的方法签名带 apiquery.ListQuery
// 并返回完整对象，而拓扑采集只需要一组名字。在装配层做一层薄适配，比为了复用
// 而让两个包互相认识更干净——topology 不应该知道 apiquery 的分页细节。
type topologyNamespaceLister struct {
	reader *kubernetes.Service
}

// VisibleNamespaces 返回该集群对当前凭据可见的 namespace 名字。
// 分页拉全量：拓扑采集要覆盖整个集群，漏掉一页会让一半工作负载的边缺失，
// 而缺失的边在图上表现为「资源孤岛」，比报错更难解释。
func (l topologyNamespaceLister) VisibleNamespaces(ctx context.Context, clusterID int64) ([]string, error) {
	const pageSize = 100
	names := make([]string, 0, pageSize)
	for page := 1; ; page++ {
		resp, err := l.reader.Namespaces(ctx, clusterID, apiquery.ListQuery{Page: page, Limit: pageSize, Ascending: true})
		if err != nil {
			return nil, err
		}
		for _, item := range resp.Items {
			names = append(names, item.Metadata.Name)
		}
		if len(resp.Items) < pageSize || resp.Remaining == 0 {
			return names, nil
		}
	}
}
