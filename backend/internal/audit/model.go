package audit

import "time"

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
}

type ListResponse struct {
	Items     []Entry `json:"items"`
	Total     int64   `json:"total"`
	Remaining int64   `json:"remaining"`
}

type ExportResult struct {
	Rows      int
	Total     int64
	Truncated bool
}
