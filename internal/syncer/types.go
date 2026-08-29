package syncer

import (
	"context"
	"time"
)

type Space struct {
	ID   string `json:"id"`
	Slug string `json:"slug"`
	Name string `json:"name"`
}

// Page is the normalized subset of a Docmost page used by the synchronizer.
// Content may be Markdown or a Docmost/ProseMirror JSON document.
type Page struct {
	ID           string    `json:"id"`
	SpaceID      string    `json:"spaceId"`
	Title        string    `json:"title"`
	SlugID       string    `json:"slugId"`
	ParentPageID string    `json:"parentPageId"`
	UpdatedAt    time.Time `json:"updatedAt"`
	Content      any       `json:"content"`
}

type Source interface {
	ListSpaces(context.Context) ([]Space, error)
	ListPages(context.Context, Space) ([]Page, error)
	GetPage(context.Context, string) (Page, error)
	Breadcrumbs(context.Context, string) ([]string, error)
}

type Sink interface {
	Write(ctx context.Context, uri, content string) error
	Delete(ctx context.Context, uri string) error
}

type Report struct {
	StartedAt         time.Time   `json:"started_at"`
	FinishedAt        time.Time   `json:"finished_at"`
	IncludedSpaces    []Space     `json:"included_spaces"`
	ExcludedSpaces    []Space     `json:"excluded_spaces"`
	UnavailableSpaces []string    `json:"unavailable_spaces"`
	Created           int         `json:"created"`
	Updated           int         `json:"updated"`
	Skipped           int         `json:"skipped"`
	Deleted           int         `json:"deleted"`
	Errors            []ItemError `json:"errors,omitempty"`
}

type ItemError struct {
	SpaceID string `json:"space_id,omitempty"`
	PageID  string `json:"page_id,omitempty"`
	Stage   string `json:"stage"`
	Message string `json:"message"`
}

func (r Report) Failed() bool { return len(r.Errors) > 0 }
