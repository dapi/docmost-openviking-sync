package syncer

import (
	"context"
	"errors"
	"testing"
)

type fakeSource struct {
	spaces   []Space
	pages    map[string][]Page
	details  map[string]Page
	failures map[string]error
}

func (s fakeSource) ListSpaces(context.Context) ([]Space, error) {
	return s.spaces, s.failures["spaces"]
}
func (s fakeSource) ListPages(_ context.Context, space Space) ([]Page, error) {
	return s.pages[space.ID], s.failures["pages:"+space.ID]
}
func (s fakeSource) GetPage(_ context.Context, id string) (Page, error) {
	return s.details[id], s.failures["page:"+id]
}
func (s fakeSource) Breadcrumbs(_ context.Context, id string) ([]string, error) {
	return []string{"Home", s.details[id].Title}, s.failures["crumb:"+id]
}

type fakeSink struct {
	writes   map[string]string
	deletes  []string
	failures map[string]error
}

func (s *fakeSink) Write(_ context.Context, u, c string) error {
	if err := s.failures["write:"+u]; err != nil {
		return err
	}
	s.writes[u] = c
	return nil
}
func (s *fakeSink) Delete(_ context.Context, u string) error {
	if err := s.failures["delete:"+u]; err != nil {
		return err
	}
	s.deletes = append(s.deletes, u)
	return nil
}
func fixture() (fakeSource, *fakeSink) {
	p := Page{ID: "p1", SpaceID: "a", Title: "One", SlugID: "one", Content: "Body"}
	return fakeSource{spaces: []Space{{ID: "a", Slug: "alpha"}, {ID: "b", Slug: "beta"}}, pages: map[string][]Page{"a": {p}, "b": {}}, details: map[string]Page{"p1": p}, failures: map[string]error{}}, &fakeSink{writes: map[string]string{}, failures: map[string]error{}}
}
func TestSyncIsIncrementalAndDeletes(t *testing.T) {
	src, sink := fixture()
	r := Reconciler{Source: src, Sink: sink, Root: "viking://user/resources/docmost", State: State{Version: 1, Pages: map[string]PageState{}}}
	first := r.Sync(context.Background())
	if first.Created != 1 || len(sink.writes) != 1 {
		t.Fatalf("first report: %+v", first)
	}
	second := r.Sync(context.Background())
	if second.Skipped != 1 || len(sink.writes) != 1 {
		t.Fatalf("second report: %+v", second)
	}
	src.details["p1"] = Page{ID: "p1", SpaceID: "a", Title: "Renamed", SlugID: "renamed", Content: "Changed"}
	updated := r.Sync(context.Background())
	if updated.Updated != 1 || len(sink.writes) != 1 {
		t.Fatalf("update report: %+v", updated)
	}
	src.pages["a"] = nil
	third := r.Sync(context.Background())
	if third.Deleted != 1 || len(sink.deletes) != 1 {
		t.Fatalf("third report: %+v", third)
	}
}
func TestFiltersAndPageFailure(t *testing.T) {
	src, sink := fixture()
	src.failures["page:p1"] = errors.New("gone temporarily")
	r := Reconciler{Source: src, Sink: sink, Root: "viking://user/resources/docmost", Allowlist: []string{"alpha", "missing"}, Denylist: []string{"beta"}, State: State{Version: 1, Pages: map[string]PageState{"p1": {SpaceID: "a", URI: "viking://user/resources/docmost/pages/p1.md"}}}}
	got := r.Sync(context.Background())
	if len(got.IncludedSpaces) != 1 || len(got.UnavailableSpaces) != 1 || got.Deleted != 0 || !got.Failed() {
		t.Fatalf("report: %+v", got)
	}
}
