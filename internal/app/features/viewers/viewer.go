// Package viewers is the data-viewer framework: one role-gated, scoped,
// filterable, pageable list mechanism, so a new event/log/data list is a
// small Viewer (a query plus row and detail mapping) rather than a new
// feature with its own routes, templates, filter parsing, paging, export,
// and scoping.
//
// The framework owns: routing (/views/{slug}, /table, /rows/{id},
// /export.csv), the per-viewer role gate, scope resolution (viewscope),
// filter rendering and parsing with URL state, cursor paging, the Live
// toggle, generic CSV export, and the standard templates. A viewer owns its
// query, columns, and how a row and its detail are built, with escape
// hatches (Cell.HTML, Detail.HTML, Exporter) for anything custom.
//
// See docs/viewers/plan.md.
package viewers

import (
	"context"
	"errors"
	"html/template"
	"io"

	"github.com/dalemusser/stratahub/internal/app/system/viewscope"
)

// ErrNotFound is returned by Viewer.Detail when no row has the given id
// within the caller's scope; the framework answers 404.
var ErrNotFound = errors.New("viewers: row not found")

// Viewer is one data list. Implementations live in the views sub-package and
// are registered at bootstrap.
type Viewer interface {
	// Slug is the URL segment, e.g. "survey-events". Lowercase, hyphenated.
	Slug() string
	// Title is the menu label and page heading.
	Title() string
	// Description is one line for the index page and the page subheading.
	Description() string
	// Roles that may open the viewer. Superadmin is always allowed.
	Roles() []string

	// Filters the framework renders and parses for this viewer (in order).
	// Organization and Group filters are framework-supplied and map to the
	// scope; do not declare them here.
	Filters() []FilterSpec
	// Columns of the table, in order. Every Row must have len(Columns) cells.
	Columns() []ColumnSpec

	// Query returns one page of rows within scope, newest first, starting
	// after the cursor (nil = first page). Implementations must apply
	// scope.Filter(...) and honor limit; they should keep Page.Next nil when
	// the page is not full.
	Query(ctx context.Context, scope *viewscope.Scope, f Filters, after *Cursor, limit int) (Page, error)
	// Detail returns the expanded view of one row, re-checking scope
	// (return ErrNotFound for ids outside it).
	Detail(ctx context.Context, scope *viewscope.Scope, id string) (*Detail, error)
}

// Exporter is an optional capability: a viewer that streams its own CSV.
// Without it, the framework exports by paging Query and writing Cell.Text.
type Exporter interface {
	Export(ctx context.Context, scope *viewscope.Scope, f Filters, w io.Writer) error
}

// Summarizer is an optional capability: header chips for the current filter
// (e.g. "Events 128 · Accepted 120 · Rejected 8 · Last 2 min ago").
type Summarizer interface {
	Summary(ctx context.Context, scope *viewscope.Scope, f Filters) ([]Chip, error)
}

// Liveable is an optional capability: enables the Live toggle, which polls
// the table on the given interval while on the first page.
type Liveable interface {
	LiveIntervalSeconds() int
}

// JSONExporter streams the current filter within scope as JSON — the
// full-fidelity download for viewers whose rows carry more than their
// columns (nested logs, snapshots). Served at /{slug}/export.json.
type JSONExporter interface {
	ExportJSON(ctx context.Context, scope *viewscope.Scope, f Filters, w io.Writer) error
}

// JSONDetailer returns one row as a JSON document for download, served at
// /{slug}/rows/{id}/export.json. Return ErrNotFound for out-of-scope ids.
type JSONDetailer interface {
	DetailJSON(ctx context.Context, scope *viewscope.Scope, id string) ([]byte, error)
}

// ColumnSpec describes one table column.
type ColumnSpec struct {
	Key   string // stable identifier (CSV header falls back to Label)
	Label string
	Class string // extra classes for the header and cells (e.g. "w-40", "whitespace-nowrap")
	Mono  bool   // render cell text in a monospace face
}

// Cell is one table cell. Text is the default rendering; Class wraps it in a
// styled span (see the Pill* constants); Href makes it a link; Title is the
// tooltip; HTML is the escape hatch for custom, already-safe markup and
// takes precedence over Text.
type Cell struct {
	Text  string
	Class string
	Title string
	Href  string
	HTML  template.HTML
}

// Row is one table row. ID must be unique within the viewer and is used for
// the detail request and the DOM id.
type Row struct {
	ID    string
	Cells []Cell
	Class string
}

// Page is one page of results.
type Page struct {
	Rows  []Row
	Next  *Cursor // nil when there are no more rows
	Total *int64  // optional count of all rows matching the filter within scope
}

// Field is one label/value pair in a detail view.
type Field struct {
	Label string
	Value string
	Mono  bool
}

// Detail is the expanded view of one row: fields, optional custom HTML, and
// the raw stored document as JSON (the "is it what I sent?" view).
type Detail struct {
	Title    string
	Subtitle string
	Fields   []Field
	RawJSON  string
	HTML     template.HTML
}

// Chip is one header summary value.
type Chip struct {
	Label string
	Value string
	Class string // optional text color classes, e.g. PillGreenText
}

// Pill classes for Cell.Class and Chip.Class. Defined in Go so Tailwind's
// content scan (which includes *.go) compiles them; every pill has light and
// dark variants. The glyph or text carries the meaning; color is secondary.
const (
	PillGray   = "inline-block px-2 py-0.5 rounded-full text-xs font-medium bg-gray-100 text-gray-700 dark:bg-gray-700 dark:text-gray-200"
	PillBlue   = "inline-block px-2 py-0.5 rounded-full text-xs font-medium bg-sky-100 text-sky-800 dark:bg-sky-900 dark:text-sky-200"
	PillIndigo = "inline-block px-2 py-0.5 rounded-full text-xs font-medium bg-indigo-100 text-indigo-800 dark:bg-indigo-900 dark:text-indigo-200"
	PillGreen  = "inline-block px-2 py-0.5 rounded-full text-xs font-medium bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200"
	PillAmber  = "inline-block px-2 py-0.5 rounded-full text-xs font-medium bg-amber-100 text-amber-800 dark:bg-amber-900 dark:text-amber-200"
	PillRed    = "inline-block px-2 py-0.5 rounded-full text-xs font-medium bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200"

	TextMuted = "text-gray-500 dark:text-gray-400"
	TextGreen = "text-green-700 dark:text-green-300"
	TextRed   = "text-red-700 dark:text-red-300"
)
