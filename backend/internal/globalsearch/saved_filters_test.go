package globalsearch

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

type savedFilterRepositoryStub struct {
	items         []SavedFilter
	created       SavedFilter
	updated       SavedFilter
	err           error
	listUserID    int64
	createItem    SavedFilter
	createLimit   int
	updateUserID  int64
	updateID      int64
	updateChanges SavedFilterChanges
	deleteUserID  int64
	deleteID      int64
}

func (s *savedFilterRepositoryStub) ListSavedFilters(_ context.Context, userID int64) ([]SavedFilter, error) {
	s.listUserID = userID
	return append([]SavedFilter(nil), s.items...), s.err
}

func (s *savedFilterRepositoryStub) CreateSavedFilter(_ context.Context, item SavedFilter, limit int) (SavedFilter, error) {
	s.createItem, s.createLimit = item, limit
	if s.err != nil {
		return SavedFilter{}, s.err
	}
	if s.created.ID == 0 {
		item.ID = 7
		return item, nil
	}
	return s.created, nil
}

func (s *savedFilterRepositoryStub) UpdateSavedFilter(_ context.Context, userID, id int64, changes SavedFilterChanges) (SavedFilter, error) {
	s.updateUserID, s.updateID, s.updateChanges = userID, id, changes
	return s.updated, s.err
}

func (s *savedFilterRepositoryStub) DeleteSavedFilter(_ context.Context, userID, id int64) error {
	s.deleteUserID, s.deleteID = userID, id
	return s.err
}

func TestSavedFilterCreateNormalizesAndForwardsOwner(t *testing.T) {
	repository := &savedFilterRepositoryStub{}
	service := NewSavedFilterService(repository)
	item, err := service.Create(context.Background(), 42, CreateSavedFilterInput{
		Name: "  Production APIs  ", Query: "  api  ", Namespace: "  prod  ",
		Kinds: []Kind{KindService, KindPod},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if item.ID != 7 || repository.createItem.UserID != 42 || repository.createLimit != MaxSavedFiltersPerUser {
		t.Fatalf("item=%#v repository=%#v", item, repository)
	}
	if repository.createItem.Name != "Production APIs" || repository.createItem.Query != "api" || repository.createItem.Namespace != "prod" {
		t.Fatalf("normalized item = %#v", repository.createItem)
	}
	if want := []Kind{KindPod, KindService}; !reflect.DeepEqual(repository.createItem.Kinds, want) {
		t.Fatalf("kinds = %#v, want %#v", repository.createItem.Kinds, want)
	}
	if repository.createItem.SchemaVersion != SavedFilterSchemaVersion || !item.Compatible {
		t.Fatalf("versioned item = %#v", item)
	}
}

func TestSavedFilterCreateRejectsInvalidShape(t *testing.T) {
	tests := []CreateSavedFilterInput{
		{Name: "", Query: "api", Kinds: []Kind{KindPod}},
		{Name: strings.Repeat("x", 41), Query: "api", Kinds: []Kind{KindPod}},
		{Name: "valid", Query: "a", Kinds: []Kind{KindPod}},
		{Name: "valid", Query: "api", Namespace: "Bad_Name", Kinds: []Kind{KindPod}},
		{Name: "valid", Query: "api", Kinds: []Kind{KindPod, KindPod}},
		{Name: "valid", Query: "api", Kinds: []Kind{"Secret"}},
	}
	for index, input := range tests {
		repository := &savedFilterRepositoryStub{}
		_, err := NewSavedFilterService(repository).Create(context.Background(), 1, input)
		if !errors.Is(err, ErrInvalidSavedFilter) {
			t.Fatalf("case %d error = %v", index, err)
		}
		if repository.createLimit != 0 {
			t.Fatalf("case %d reached repository", index)
		}
	}
	if _, err := NewSavedFilterService(&savedFilterRepositoryStub{}).Create(context.Background(), 0, CreateSavedFilterInput{Name: "valid", Query: "api"}); !errors.Is(err, ErrInvalidSavedFilter) {
		t.Fatalf("invalid owner error = %v", err)
	}
}

func TestSavedFilterCreatePreservesRepositoryConflicts(t *testing.T) {
	for _, repositoryError := range []error{ErrSavedFilterLimit, ErrSavedFilterNameExists, errors.New("database unavailable")} {
		_, err := NewSavedFilterService(&savedFilterRepositoryStub{err: repositoryError}).Create(context.Background(), 1, CreateSavedFilterInput{Name: "valid", Query: "api"})
		if !errors.Is(err, repositoryError) {
			t.Fatalf("error = %v, want %v", err, repositoryError)
		}
	}
}

func TestSavedFilterUpdateSupportsRenameOrCompleteOverwrite(t *testing.T) {
	name := "  Renamed  "
	repository := &savedFilterRepositoryStub{updated: SavedFilter{ID: 9, UserID: 3, Name: "Renamed", Query: "api", Kinds: []Kind{KindPod}, SchemaVersion: 1}}
	service := NewSavedFilterService(repository)
	if _, err := service.Update(context.Background(), 3, 9, UpdateSavedFilterInput{Name: &name}); err != nil {
		t.Fatalf("rename error = %v", err)
	}
	if repository.updateUserID != 3 || repository.updateID != 9 || repository.updateChanges.Name == nil || *repository.updateChanges.Name != "Renamed" {
		t.Fatalf("rename forwarding = %#v", repository)
	}

	query, namespace, kinds := " worker ", " jobs ", []Kind{KindIngress, KindDeployment}
	if _, err := service.Update(context.Background(), 3, 9, UpdateSavedFilterInput{Query: &query, Namespace: &namespace, Kinds: &kinds}); err != nil {
		t.Fatalf("overwrite error = %v", err)
	}
	changes := repository.updateChanges
	if changes.Query == nil || *changes.Query != "worker" || changes.Namespace == nil || *changes.Namespace != "jobs" || changes.SchemaVersion == nil || *changes.SchemaVersion != 1 {
		t.Fatalf("overwrite changes = %#v", changes)
	}
	if want := []Kind{KindDeployment, KindIngress}; changes.Kinds == nil || !reflect.DeepEqual(*changes.Kinds, want) {
		t.Fatalf("overwrite kinds = %#v, want %#v", changes.Kinds, want)
	}
}

func TestSavedFilterUpdateRejectsPartialQueryAndEmptyPatch(t *testing.T) {
	query := "api"
	for _, input := range []UpdateSavedFilterInput{{}, {Query: &query}} {
		repository := &savedFilterRepositoryStub{}
		_, err := NewSavedFilterService(repository).Update(context.Background(), 1, 2, input)
		if !errors.Is(err, ErrInvalidSavedFilter) || repository.updateID != 0 {
			t.Fatalf("input=%#v error=%v repository=%#v", input, err, repository)
		}
	}
}

func TestSavedFilterListMarksIncompatibleRecordsWithoutFailing(t *testing.T) {
	repository := &savedFilterRepositoryStub{items: []SavedFilter{
		{ID: 1, Query: "api", Kinds: []Kind{KindPod}, SchemaVersion: SavedFilterSchemaVersion},
		{ID: 2, Query: "api", Kinds: []Kind{KindPod}, SchemaVersion: SavedFilterSchemaVersion + 1},
		{ID: 3, Query: "a", Kinds: []Kind{KindPod}, SchemaVersion: SavedFilterSchemaVersion},
	}}
	response, err := NewSavedFilterService(repository).List(context.Background(), 11)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if repository.listUserID != 11 || response.Total != 3 || response.Limit != 20 {
		t.Fatalf("response=%#v repository=%#v", response, repository)
	}
	if !response.Items[0].Compatible || response.Items[1].IncompatibilityCode != IncompatibleSchemaVersion || response.Items[2].IncompatibilityCode != IncompatibleQueryShape {
		t.Fatalf("compatibility projection = %#v", response.Items)
	}
}

func TestSavedFilterDeleteForwardsOwnershipAndNotFound(t *testing.T) {
	repository := &savedFilterRepositoryStub{err: ErrSavedFilterNotFound}
	err := NewSavedFilterService(repository).Delete(context.Background(), 8, 12)
	if !errors.Is(err, ErrSavedFilterNotFound) || repository.deleteUserID != 8 || repository.deleteID != 12 {
		t.Fatalf("error=%v repository=%#v", err, repository)
	}
}
