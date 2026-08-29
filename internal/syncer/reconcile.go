package syncer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"
)

type Reconciler struct {
	Source     Source
	Sink       Sink
	Root       string
	DocmostURL string
	Allowlist  []string
	Denylist   []string
	State      State
}

func (r *Reconciler) Sync(ctx context.Context) Report {
	report := Report{StartedAt: time.Now().UTC()}
	defer func() { report.FinishedAt = time.Now().UTC() }()
	spaces, err := r.Source.ListSpaces(ctx)
	if err != nil {
		report.Errors = append(report.Errors, ItemError{Stage: "list_spaces", Message: err.Error()})
		return report
	}
	included, excluded, unavailable := filterSpaces(spaces, r.Allowlist, r.Denylist)
	report.IncludedSpaces, report.ExcludedSpaces, report.UnavailableSpaces = included, excluded, unavailable
	seen := make(map[string]bool)
	successfulSpaces := make(map[string]bool)
	for _, space := range included {
		pages, err := r.Source.ListPages(ctx, space)
		if err != nil {
			report.Errors = append(report.Errors, ItemError{SpaceID: space.ID, Stage: "list_pages", Message: err.Error()})
			continue
		}
		successfulSpaces[space.ID] = true
		for _, listed := range pages {
			// A listed page is retained even when its details cannot be fetched.
			seen[listed.ID] = true
			page, err := r.Source.GetPage(ctx, listed.ID)
			if err != nil {
				report.Errors = append(report.Errors, ItemError{SpaceID: space.ID, PageID: listed.ID, Stage: "get_page", Message: err.Error()})
				continue
			}
			if page.SpaceID == "" {
				page.SpaceID = space.ID
			}
			breadcrumbs, err := r.Source.Breadcrumbs(ctx, page.ID)
			if err != nil {
				report.Errors = append(report.Errors, ItemError{SpaceID: space.ID, PageID: page.ID, Stage: "breadcrumbs", Message: err.Error()})
				continue
			}
			content, err := document(r.DocmostURL, space, page, breadcrumbs)
			if err != nil {
				report.Errors = append(report.Errors, ItemError{SpaceID: space.ID, PageID: page.ID, Stage: "render", Message: err.Error()})
				continue
			}
			uri := pageURI(r.Root, page.ID)
			fingerprint := digest(content)
			old, exists := r.State.Pages[page.ID]
			if exists && old.Fingerprint == fingerprint && old.URI == uri {
				report.Skipped++
				continue
			}
			if err := r.Sink.Write(ctx, uri, content); err != nil {
				report.Errors = append(report.Errors, ItemError{SpaceID: space.ID, PageID: page.ID, Stage: "write", Message: err.Error()})
				continue
			}
			r.State.Pages[page.ID] = PageState{SpaceID: space.ID, URI: uri, Fingerprint: fingerprint}
			if exists {
				report.Updated++
			} else {
				report.Created++
			}
		}
	}
	// Delete only from spaces whose page listing completed successfully. This
	// makes a transient listing error incapable of deleting valid resources.
	for pageID, old := range r.State.Pages {
		if successfulSpaces[old.SpaceID] && !seen[pageID] {
			if err := r.Sink.Delete(ctx, old.URI); err != nil {
				report.Errors = append(report.Errors, ItemError{SpaceID: old.SpaceID, PageID: pageID, Stage: "delete", Message: err.Error()})
				continue
			}
			delete(r.State.Pages, pageID)
			report.Deleted++
		}
	}
	return report
}

func filterSpaces(spaces []Space, allow, deny []string) (included, excluded []Space, unavailable []string) {
	allowSet, denySet := makeSet(allow), makeSet(deny)
	found := make(map[string]bool)
	for _, space := range spaces {
		matchesAllow := len(allowSet) == 0 || allowSet[space.ID] || allowSet[space.Slug]
		matchesDeny := denySet[space.ID] || denySet[space.Slug]
		if allowSet[space.ID] || allowSet[space.Slug] {
			found[space.ID], found[space.Slug] = true, true
		}
		if matchesAllow && !matchesDeny {
			included = append(included, space)
		} else {
			excluded = append(excluded, space)
		}
	}
	for id := range allowSet {
		if !found[id] {
			unavailable = append(unavailable, id)
		}
	}
	sort.Strings(unavailable)
	return
}

func makeSet(values []string) map[string]bool {
	out := make(map[string]bool)
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			out[value] = true
		}
	}
	return out
}
func pageURI(root, pageID string) string {
	return strings.TrimSuffix(root, "/") + "/pages/" + url.PathEscape(pageID) + ".md"
}
func digest(s string) string { sum := sha256.Sum256([]byte(s)); return hex.EncodeToString(sum[:]) }

func document(docmostURL string, space Space, page Page, breadcrumbs []string) (string, error) {
	body, err := RenderMarkdown(page.Content)
	if err != nil {
		return "", err
	}
	canonical := ""
	if page.SlugID != "" {
		canonical = fmt.Sprintf("%s/s/%s/p/%s", strings.TrimSuffix(strings.TrimSuffix(docmostURL, "/"), "/api"), url.PathEscape(space.Slug), url.PathEscape(page.SlugID))
	}
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "docmost_page_id: %q\nspace_id: %q\nspace_slug: %q\ntitle: %q\n", page.ID, space.ID, space.Slug, page.Title)
	fmt.Fprintf(&b, "canonical_url: %q\nupdated_at: %q\n", canonical, page.UpdatedAt.UTC().Format(time.RFC3339))
	if len(breadcrumbs) > 0 {
		fmt.Fprintf(&b, "breadcrumb: %q\n", breadcrumbs)
	}
	b.WriteString("---\n\n# " + page.Title + "\n\n")
	b.WriteString(body)
	return b.String(), nil
}
