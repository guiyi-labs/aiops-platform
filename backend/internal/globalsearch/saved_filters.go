package globalsearch

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"
)

const (
	SavedFilterSchemaVersion = 1
	MaxSavedFiltersPerUser   = 20

	IncompatibleSchemaVersion = "SCHEMA_VERSION"
	IncompatibleQueryShape    = "QUERY_SHAPE"
)

var (
	ErrInvalidSavedFilter    = errors.New("saved global search filter is invalid")
	ErrSavedFilterLimit      = errors.New("saved global search filter limit reached")
	ErrSavedFilterNameExists = errors.New("saved global search filter name already exists")
	ErrSavedFilterNotFound   = errors.New("saved global search filter not found")
)

type SavedFilter struct {
	ID                  int64  `json:"id"`
	UserID              int64  `json:"-"`
	Name                string `json:"name"`
	Query               string `json:"query"`
	Namespace           string `json:"namespace,omitempty"`
	Kinds               []Kind `json:"kinds"`
	SchemaVersion       int    `json:"schema_version"`
	Compatible          bool   `json:"compatible"`
	IncompatibilityCode string `json:"incompatibility_code,omitempty"`
	CreatedAt           string `json:"created_at"`
	UpdatedAt           string `json:"updated_at"`
}

type SavedFilterChanges struct {
	Name          *string
	Query         *string
	Namespace     *string
	Kinds         *[]Kind
	SchemaVersion *int
}

type SavedFilterRepository interface {
	ListSavedFilters(context.Context, int64) ([]SavedFilter, error)
	CreateSavedFilter(context.Context, SavedFilter, int) (SavedFilter, error)
	UpdateSavedFilter(context.Context, int64, int64, SavedFilterChanges) (SavedFilter, error)
	DeleteSavedFilter(context.Context, int64, int64) error
}

type CreateSavedFilterInput struct {
	Name      string
	Query     string
	Namespace string
	Kinds     []Kind
}

type UpdateSavedFilterInput struct {
	Name      *string
	Query     *string
	Namespace *string
	Kinds     *[]Kind
}

type SavedFilterListResponse struct {
	Items []SavedFilter `json:"items"`
	Total int           `json:"total"`
	Limit int           `json:"limit"`
}

type SavedFilterService struct{ repository SavedFilterRepository }

func NewSavedFilterService(repository SavedFilterRepository) *SavedFilterService {
	return &SavedFilterService{repository: repository}
}

func (s *SavedFilterService) List(ctx context.Context, userID int64) (SavedFilterListResponse, error) {
	if userID < 1 {
		return SavedFilterListResponse{}, ErrInvalidSavedFilter
	}
	items, err := s.repository.ListSavedFilters(ctx, userID)
	if err != nil {
		return SavedFilterListResponse{}, err
	}
	for index := range items {
		items[index] = characterizeSavedFilter(items[index])
	}
	return SavedFilterListResponse{Items: items, Total: len(items), Limit: MaxSavedFiltersPerUser}, nil
}

func (s *SavedFilterService) Create(ctx context.Context, userID int64, input CreateSavedFilterInput) (SavedFilter, error) {
	name, query, namespace, kinds, err := normalizeSavedFilter(input.Name, input.Query, input.Namespace, input.Kinds)
	if userID < 1 || err != nil {
		return SavedFilter{}, ErrInvalidSavedFilter
	}
	item, err := s.repository.CreateSavedFilter(ctx, SavedFilter{
		UserID: userID, Name: name, Query: query, Namespace: namespace, Kinds: kinds,
		SchemaVersion: SavedFilterSchemaVersion, Compatible: true,
	}, MaxSavedFiltersPerUser)
	if err != nil {
		return SavedFilter{}, err
	}
	return characterizeSavedFilter(item), nil
}

func (s *SavedFilterService) Update(ctx context.Context, userID, id int64, input UpdateSavedFilterInput) (SavedFilter, error) {
	if userID < 1 || id < 1 || (input.Name == nil && input.Query == nil && input.Namespace == nil && input.Kinds == nil) {
		return SavedFilter{}, ErrInvalidSavedFilter
	}
	changes := SavedFilterChanges{}
	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if !validSavedFilterName(name) {
			return SavedFilter{}, ErrInvalidSavedFilter
		}
		changes.Name = &name
	}
	queryFields := 0
	if input.Query != nil {
		queryFields++
	}
	if input.Namespace != nil {
		queryFields++
	}
	if input.Kinds != nil {
		queryFields++
	}
	if queryFields != 0 {
		if queryFields != 3 {
			return SavedFilter{}, ErrInvalidSavedFilter
		}
		query, namespace, kinds, err := normalizeSavedFilterQuery(*input.Query, *input.Namespace, *input.Kinds)
		if err != nil {
			return SavedFilter{}, err
		}
		version := SavedFilterSchemaVersion
		changes.Query, changes.Namespace, changes.Kinds, changes.SchemaVersion = &query, &namespace, &kinds, &version
	}
	item, err := s.repository.UpdateSavedFilter(ctx, userID, id, changes)
	if err != nil {
		return SavedFilter{}, err
	}
	return characterizeSavedFilter(item), nil
}

func (s *SavedFilterService) Delete(ctx context.Context, userID, id int64) error {
	if userID < 1 || id < 1 {
		return ErrInvalidSavedFilter
	}
	return s.repository.DeleteSavedFilter(ctx, userID, id)
}

func normalizeSavedFilter(name, query, namespace string, kinds []Kind) (string, string, string, []Kind, error) {
	name = strings.TrimSpace(name)
	query, namespace, kinds, err := normalizeSavedFilterQuery(query, namespace, kinds)
	if !validSavedFilterName(name) || err != nil {
		return "", "", "", nil, ErrInvalidSavedFilter
	}
	return name, query, namespace, kinds, nil
}

func normalizeSavedFilterQuery(query, namespace string, kinds []Kind) (string, string, []Kind, error) {
	query, namespace = strings.TrimSpace(query), strings.TrimSpace(namespace)
	if len(query) < 2 || len(query) > 64 || len(namespace) > 63 || (namespace != "" && !namespaceName.MatchString(namespace)) {
		return "", "", nil, ErrInvalidSavedFilter
	}
	if len(kinds) == 0 {
		kinds = SupportedKinds()
	}
	seen := map[Kind]bool{}
	for _, kind := range kinds {
		if seen[kind] || kindIndex(kind) == len(supportedKinds) {
			return "", "", nil, ErrInvalidSavedFilter
		}
		seen[kind] = true
	}
	canonical := make([]Kind, 0, len(seen))
	for _, kind := range supportedKinds {
		if seen[kind] {
			canonical = append(canonical, kind)
		}
	}
	return query, namespace, canonical, nil
}

func validSavedFilterName(value string) bool {
	length := utf8.RuneCountInString(value)
	return length >= 1 && length <= 40
}

func characterizeSavedFilter(item SavedFilter) SavedFilter {
	item.Compatible, item.IncompatibilityCode = true, ""
	if item.SchemaVersion != SavedFilterSchemaVersion {
		item.Compatible, item.IncompatibilityCode = false, IncompatibleSchemaVersion
		return item
	}
	query, namespace, kinds, err := normalizeSavedFilterQuery(item.Query, item.Namespace, item.Kinds)
	if err != nil {
		item.Compatible, item.IncompatibilityCode = false, IncompatibleQueryShape
		return item
	}
	item.Query, item.Namespace, item.Kinds = query, namespace, kinds
	return item
}
