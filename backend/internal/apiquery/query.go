package apiquery

import (
	"fmt"
	"net/http"
	"strconv"
)

const (
	defaultPage  = 1
	defaultLimit = 20
	maxLimit     = 100
)

type ListQuery struct {
	Page          int
	Limit         int
	Offset        int
	SortBy        string
	Ascending     bool
	Name          string
	LabelSelector string
	FieldSelector string
}

type ListResponse[T any] struct {
	Items     []T `json:"items"`
	Total     int `json:"total"`
	Remaining int `json:"remaining"`
}

func Parse(request *http.Request, allowedSortFields ...string) (ListQuery, error) {
	values := request.URL.Query()

	page, err := positiveInt(values.Get("page"), defaultPage)
	if err != nil {
		return ListQuery{}, fmt.Errorf("invalid page: %w", err)
	}
	limit, err := positiveInt(values.Get("limit"), defaultLimit)
	if err != nil {
		return ListQuery{}, fmt.Errorf("invalid limit: %w", err)
	}
	if limit > maxLimit {
		return ListQuery{}, fmt.Errorf("invalid limit: must not exceed %d", maxLimit)
	}

	ascending := false
	if raw := values.Get("ascending"); raw != "" {
		ascending, err = strconv.ParseBool(raw)
		if err != nil {
			return ListQuery{}, fmt.Errorf("invalid ascending: must be true or false")
		}
	}

	sortBy := values.Get("sort_by")
	if sortBy != "" && !contains(allowedSortFields, sortBy) {
		return ListQuery{}, fmt.Errorf("invalid sort_by: field %q is not supported", sortBy)
	}

	return ListQuery{
		Page:          page,
		Limit:         limit,
		Offset:        (page - 1) * limit,
		SortBy:        sortBy,
		Ascending:     ascending,
		Name:          values.Get("name"),
		LabelSelector: values.Get("label_selector"),
		FieldSelector: values.Get("field_selector"),
	}, nil
}

func positiveInt(raw string, fallback int) (int, error) {
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 {
		return 0, fmt.Errorf("must be a positive integer")
	}
	return value, nil
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
