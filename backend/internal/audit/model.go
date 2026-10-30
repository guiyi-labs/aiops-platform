package audit

import "time"

const MaxArchiveRecords = 10000

type Actor struct {
	ID   int64  `json:"id,omitempty"`
	Name string `json:"name"`
}

type ResourceRef struct {
	Type      string `json:"type,omitempty"`
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name,omitempty"`
}

type Entry struct {
	ID         int64          `json:"id"`
	Actor      Actor          `json:"actor"`
	ClusterID  *int64         `json:"cluster_id,omitempty"`
	Action     string         `json:"action"`
	Resource   ResourceRef    `json:"resource"`
	Result     string         `json:"result"`
	RequestID  string         `json:"request_id"`
	StatusCode int            `json:"status_code"`
	IPAddress  string         `json:"ip_address"`
	UserAgent  string         `json:"user_agent"`
	Details    map[string]any `json:"details"`
	CreatedAt  time.Time      `json:"created_at"`
}

type Filter struct {
	ClusterID int64
	Action    string
	Result    string
	Limit     int
	// BeforeID 是游标分页的锚点：只返回 id 小于它的记录。
	//
	// 为什么不用 OFFSET：审计表是只追加的高频表，翻页期间新记录会插到最前面，
	// OFFSET 会让相邻两页出现重复或遗漏；而且 ORDER BY created_at DESC 的深
	// OFFSET 每页都要扫过前面所有行。按自增 id 倒序取游标既稳定又走索引。
	BeforeID int64
}

type ListResponse struct {
	Items     []Entry `json:"items"`
	Total     int64   `json:"total"`
	Remaining int64   `json:"remaining"`
	// NextBeforeID 是下一页的游标（本页最后一条的 id）。为 0 表示没有更多。
	NextBeforeID int64 `json:"next_before_id,omitempty"`
}

type ExportResult struct {
	Rows      int
	Total     int64
	Truncated bool
}
