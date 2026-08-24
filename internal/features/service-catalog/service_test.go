package servicecatalog

import (
	"context"
	"errors"
	"testing"
	"time"
)

// fakeStore is an in-memory Store keyed by id, enough to exercise the business
// rules without a database.
type fakeStore struct {
	byID      map[string]*Service
	codes     map[int64]bool
	nextCode  int64
	failWith  error
	lastNew   NewService
	lastPatch Changes
}

func newFakeStore(seed ...*Service) *fakeStore {
	s := &fakeStore{byID: map[string]*Service{}, codes: map[int64]bool{}, nextCode: 100}
	for _, service := range seed {
		s.byID[service.ID] = service
		s.codes[service.Code] = true
	}
	return s
}

func (s *fakeStore) Create(_ context.Context, in NewService) (*Service, error) {
	s.lastNew = in
	if s.failWith != nil {
		return nil, s.failWith
	}
	code := s.nextCode
	s.nextCode++
	if in.Code != nil {
		code = *in.Code
	}
	if s.codes[code] {
		return nil, ErrCodeAlreadyExists
	}
	s.codes[code] = true
	created := &Service{
		ID:            "svc-" + in.Name,
		Code:          code,
		Name:          in.Name,
		Description:   in.Description,
		Price:         in.Price,
		EstimatedTime: in.EstimatedTime,
		Active:        true,
		CreatedAt:     time.Unix(0, 0).UTC(),
		UpdatedAt:     time.Unix(0, 0).UTC(),
	}
	s.byID[created.ID] = created
	return created, nil
}

func (s *fakeStore) List(_ context.Context, filter ListFilter) ([]Service, error) {
	if s.failWith != nil {
		return nil, s.failWith
	}
	services := []Service{}
	for _, service := range s.byID {
		if filter.Active != nil && service.Active != *filter.Active {
			continue
		}
		services = append(services, *service)
	}
	return services, nil
}

func (s *fakeStore) FindByID(_ context.Context, id string) (*Service, error) {
	if s.failWith != nil {
		return nil, s.failWith
	}
	if service, ok := s.byID[id]; ok {
		return service, nil
	}
	return nil, ErrServiceNotFound
}

func (s *fakeStore) Update(_ context.Context, id string, changes Changes) (*Service, error) {
	s.lastPatch = changes
	if s.failWith != nil {
		return nil, s.failWith
	}
	service, ok := s.byID[id]
	if !ok {
		return nil, ErrServiceNotFound
	}
	if changes.Name != nil {
		service.Name = *changes.Name
	}
	if changes.Description != nil {
		service.Description = *changes.Description
	}
	if changes.Price != nil {
		service.Price = *changes.Price
	}
	if changes.EstimatedTime != nil {
		service.EstimatedTime = changes.EstimatedTime
	}
	return service, nil
}

func (s *fakeStore) Deactivate(_ context.Context, id string) error {
	if s.failWith != nil {
		return s.failWith
	}
	service, ok := s.byID[id]
	if !ok {
		return ErrServiceNotFound
	}
	service.Active = false
	return nil
}

func seededService() *Service {
	return &Service{ID: "11111111-1111-1111-1111-111111111111", Code: 10, Name: "Oil Change", Price: 80, Active: true}
}

func ptr[T any](v T) *T { return &v }

// FR1/AC1: a service can be registered with code, name, and price.
func TestCreateWithCodeNameAndPrice(t *testing.T) {
	catalog := NewCatalog(newFakeStore())
	created, err := catalog.Create(context.Background(), NewService{Code: ptr(int64(1001)), Name: "Oil Change", Price: 150})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.Code != 1001 || created.Name != "Oil Change" || created.Price != 150 || !created.Active {
		t.Fatalf("created = %+v", created)
	}
}

// D1: without a code, the store generates one.
func TestCreateWithoutCode(t *testing.T) {
	catalog := NewCatalog(newFakeStore())
	created, err := catalog.Create(context.Background(), NewService{Name: "Wheel Alignment", Price: 90})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.Code == 0 {
		t.Fatal("code was not generated")
	}
}

// BR1: name is required, blanks included.
func TestCreateRequiresName(t *testing.T) {
	catalog := NewCatalog(newFakeStore())
	for _, name := range []string{"", "   "} {
		_, err := catalog.Create(context.Background(), NewService{Name: name, Price: 10})
		var validation ValidationError
		if !errors.As(err, &validation) {
			t.Fatalf("name %q: err = %v, want ValidationError", name, err)
		}
	}
}

// BR3/AC3: a negative price is rejected.
func TestCreateRejectsNegativePrice(t *testing.T) {
	catalog := NewCatalog(newFakeStore())
	_, err := catalog.Create(context.Background(), NewService{Name: "Oil Change", Price: -0.01})
	var validation ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("err = %v, want ValidationError", err)
	}
}

// BR4/AC4: estimated time, when informed, must be greater than zero.
func TestCreateRejectsNonPositiveEstimatedTime(t *testing.T) {
	catalog := NewCatalog(newFakeStore())
	for _, estimated := range []int{0, -30} {
		_, err := catalog.Create(context.Background(), NewService{Name: "Oil Change", Price: 10, EstimatedTime: ptr(estimated)})
		var validation ValidationError
		if !errors.As(err, &validation) {
			t.Fatalf("estimated_time %d: err = %v, want ValidationError", estimated, err)
		}
	}
}

func TestCreateAcceptsAbsentEstimatedTime(t *testing.T) {
	catalog := NewCatalog(newFakeStore())
	created, err := catalog.Create(context.Background(), NewService{Name: "Diagnostics", Price: 60})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.EstimatedTime != nil {
		t.Fatalf("estimatedTime = %v, want nil", *created.EstimatedTime)
	}
}

func TestCreateRejectsNonPositiveCode(t *testing.T) {
	catalog := NewCatalog(newFakeStore())
	_, err := catalog.Create(context.Background(), NewService{Code: ptr(int64(0)), Name: "Oil Change", Price: 10})
	var validation ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("err = %v, want ValidationError", err)
	}
}

func TestCreateTrimsNameAndDescription(t *testing.T) {
	store := newFakeStore()
	catalog := NewCatalog(store)
	if _, err := catalog.Create(context.Background(), NewService{Name: "  Oil Change  ", Description: "  cleans  ", Price: 10}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if store.lastNew.Name != "Oil Change" || store.lastNew.Description != "cleans" {
		t.Fatalf("stored = %+v", store.lastNew)
	}
}

// BR2/AC2: a duplicate code is reported as such, not swallowed.
func TestCreateDuplicateCode(t *testing.T) {
	catalog := NewCatalog(newFakeStore(seededService()))
	_, err := catalog.Create(context.Background(), NewService{Code: ptr(int64(10)), Name: "Other", Price: 10})
	if !errors.Is(err, ErrCodeAlreadyExists) {
		t.Fatalf("err = %v, want ErrCodeAlreadyExists", err)
	}
}

// FR2/AC5: the listing distinguishes active from inactive services.
func TestListFilteredByActive(t *testing.T) {
	inactive := &Service{ID: "22222222-2222-2222-2222-222222222222", Code: 11, Name: "Retired", Active: false}
	catalog := NewCatalog(newFakeStore(seededService(), inactive))

	all, err := catalog.List(context.Background(), ListFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("unfiltered list = %d services, want 2", len(all))
	}

	onlyActive, err := catalog.List(context.Background(), ListFilter{Active: ptr(true)})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(onlyActive) != 1 || !onlyActive[0].Active {
		t.Fatalf("active list = %+v", onlyActive)
	}
}

func TestByIDNotFound(t *testing.T) {
	catalog := NewCatalog(newFakeStore())
	_, err := catalog.ByID(context.Background(), "33333333-3333-3333-3333-333333333333")
	if !errors.Is(err, ErrServiceNotFound) {
		t.Fatalf("err = %v, want ErrServiceNotFound", err)
	}
}

// FR4/AC6: description, price, and estimated time can be updated.
func TestUpdateFields(t *testing.T) {
	service := seededService()
	catalog := NewCatalog(newFakeStore(service))
	updated, err := catalog.Update(context.Background(), service.ID, Changes{
		Description:   ptr("Full synthetic oil."),
		Price:         ptr(180.5),
		EstimatedTime: ptr(45),
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Description != "Full synthetic oil." || updated.Price != 180.5 || *updated.EstimatedTime != 45 {
		t.Fatalf("updated = %+v", updated)
	}
}

func TestUpdateValidations(t *testing.T) {
	service := seededService()
	catalog := NewCatalog(newFakeStore(service))
	cases := map[string]Changes{
		"empty change set":    {},
		"blank name":          {Name: ptr("   ")},
		"negative price":      {Price: ptr(-1.0)},
		"zero estimated time": {EstimatedTime: ptr(0)},
	}
	for name, changes := range cases {
		_, err := catalog.Update(context.Background(), service.ID, changes)
		var validation ValidationError
		if !errors.As(err, &validation) {
			t.Fatalf("%s: err = %v, want ValidationError", name, err)
		}
	}
}

func TestUpdateNotFound(t *testing.T) {
	catalog := NewCatalog(newFakeStore())
	_, err := catalog.Update(context.Background(), "44444444-4444-4444-4444-444444444444", Changes{Price: ptr(10.0)})
	if !errors.Is(err, ErrServiceNotFound) {
		t.Fatalf("err = %v, want ErrServiceNotFound", err)
	}
}

// FR5/AC7/BR7: deletion is logical — the record survives, flagged inactive.
func TestDeactivateKeepsRecord(t *testing.T) {
	service := seededService()
	store := newFakeStore(service)
	catalog := NewCatalog(store)

	if err := catalog.Deactivate(context.Background(), service.ID); err != nil {
		t.Fatalf("Deactivate: %v", err)
	}
	found, err := catalog.ByID(context.Background(), service.ID)
	if err != nil {
		t.Fatalf("ByID after deactivate: %v", err)
	}
	if found.Active {
		t.Fatal("service is still active after deactivation")
	}
}

func TestDeactivateNotFound(t *testing.T) {
	catalog := NewCatalog(newFakeStore())
	err := catalog.Deactivate(context.Background(), "55555555-5555-5555-5555-555555555555")
	if !errors.Is(err, ErrServiceNotFound) {
		t.Fatalf("err = %v, want ErrServiceNotFound", err)
	}
}
