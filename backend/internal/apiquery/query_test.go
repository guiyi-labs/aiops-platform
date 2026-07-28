package apiquery

import (
	"net/http/httptest"
	"testing"
)

func TestParseDefaults(t *testing.T) {
	request := httptest.NewRequest("GET", "/pods", nil)

	query, err := Parse(request, "name", "created_at")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if query.Page != 1 || query.Limit != 20 || query.Offset != 0 {
		t.Fatalf("Parse() pagination = page %d limit %d offset %d", query.Page, query.Limit, query.Offset)
	}
}

func TestParseValues(t *testing.T) {
	request := httptest.NewRequest("GET", "/pods?page=3&limit=25&sort_by=name&ascending=true&name=api&label_selector=app%3Dapi", nil)

	query, err := Parse(request, "name")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if query.Offset != 50 || query.SortBy != "name" || !query.Ascending || query.Name != "api" || query.LabelSelector != "app=api" {
		t.Fatalf("Parse() = %#v", query)
	}
}

func TestParseRejectsInvalidValues(t *testing.T) {
	tests := []string{
		"/pods?page=0",
		"/pods?limit=101",
		"/pods?ascending=sometimes",
		"/pods?sort_by=secret",
	}

	for _, target := range tests {
		t.Run(target, func(t *testing.T) {
			request := httptest.NewRequest("GET", target, nil)
			if _, err := Parse(request, "name"); err == nil {
				t.Fatal("Parse() error = nil, want validation error")
			}
		})
	}
}
