// internal/app/system/paging/paging.go
package paging

import (
	"net/http"
	"strconv"

	"github.com/dalemusser/waffle/pantry/query"
	wafflemongo "github.com/dalemusser/waffle/pantry/mongo"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// PageSize is the default number of rows shown in paged lists.
// Keep this as an int because most call sites add/subtract and then
// cast to int64 for Mongo Find().SetLimit().
const PageSize = 50

// ModalPageSize is a smaller page size for modal pickers where less
// vertical space is available.
const ModalPageSize = 10

// LimitPlusOne returns PageSize+1 as int64 for look‑ahead pagination
// (fetch one extra document to detect hasNext).
func LimitPlusOne() int64 { return int64(PageSize + 1) }

// ModalLimitPlusOne returns ModalPageSize+1 as int64 for look‑ahead pagination
// in modal pickers.
func ModalLimitPlusOne() int64 { return int64(ModalPageSize + 1) }

// ParseStart extracts the human-friendly "start" query parameter (1-based index).
// Returns 1 if not present or invalid.
func ParseStart(r *http.Request) int {
	s := query.Get(r, "start")
	if s == "" {
		return 1
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return 1
	}
	return n
}

// Result holds the output of TrimPage for keyset pagination.
type Result struct {
	HasPrev bool
	HasNext bool
}

// TrimPage trims a fetched slice for keyset pagination.
// Call this after fetching PageSize+1 rows. It modifies the slice in place
// and returns pagination indicators.
//
// When going backwards (before != ""):
//   - If len > PageSize, trim the first element (older page exists)
//   - HasNext is always true (we came from somewhere)
//
// When going forwards or on first page:
//   - If len > PageSize, trim to PageSize (next page exists)
//   - HasPrev is true only if after != ""
func TrimPage[T any](rows *[]T, before, after string) Result {
	return trimPageWithSize(rows, before, after, PageSize)
}

// TrimPageModal is like TrimPage but uses ModalPageSize.
func TrimPageModal[T any](rows *[]T, before, after string) Result {
	return trimPageWithSize(rows, before, after, ModalPageSize)
}

// trimPageWithSize is the internal implementation that accepts a custom page size.
func trimPageWithSize[T any](rows *[]T, before, after string, pageSize int) Result {
	orig := len(*rows)
	var hasPrev, hasNext bool

	if before != "" {
		if orig > pageSize {
			*rows = (*rows)[1:]
			hasPrev = true
		}
		hasNext = true
	} else {
		if orig > pageSize {
			*rows = (*rows)[:pageSize]
			hasNext = true
		}
		hasPrev = after != ""
	}

	return Result{HasPrev: hasPrev, HasNext: hasNext}
}

// Range holds computed display range values for a paginated list.
type Range struct {
	Start     int // 1-based start index (0 if no results)
	End       int // 1-based end index (0 if no results)
	PrevStart int // start value for previous page link
	NextStart int // start value for next page link
}

// ComputeRange calculates display range values given the current start index
// and number of items shown.
func ComputeRange(start, shown int) Range {
	return computeRangeWithSize(start, shown, PageSize)
}

// ComputeRangeModal is like ComputeRange but uses ModalPageSize.
func ComputeRangeModal(start, shown int) Range {
	return computeRangeWithSize(start, shown, ModalPageSize)
}

// computeRangeWithSize is the internal implementation that accepts a custom page size.
func computeRangeWithSize(start, shown, pageSize int) Range {
	if shown == 0 {
		return Range{Start: 0, End: 0, PrevStart: 1, NextStart: 1}
	}

	prevStart := start - pageSize
	if prevStart < 1 {
		prevStart = 1
	}

	return Range{
		Start:     start,
		End:       start + shown - 1,
		PrevStart: prevStart,
		NextStart: start + shown,
	}
}

// Direction indicates the pagination direction.
type Direction int

const (
	Forward  Direction = iota // Default: sort ascending, use "gt" for cursor
	Backward                  // Sort descending, use "lt" for cursor
)

// KeysetConfig holds the result of configuring keyset pagination.
type KeysetConfig struct {
	Direction Direction
	SortOrder int                // 1 for ascending, -1 for descending
	Cursor    *wafflemongo.Cursor
}

// ConfigureKeyset determines pagination direction and decodes the cursor.
// Returns the config to use for building the query.
func ConfigureKeyset(before, after string) KeysetConfig {
	cfg := KeysetConfig{
		Direction: Forward,
		SortOrder: 1,
	}

	if before != "" {
		cfg.Direction = Backward
		cfg.SortOrder = -1
		if c, ok := wafflemongo.DecodeCursor(before); ok {
			cfg.Cursor = &c
		}
	} else if after != "" {
		if c, ok := wafflemongo.DecodeCursor(after); ok {
			cfg.Cursor = &c
		}
	}

	return cfg
}

// ApplyToFind configures FindOptions with sort and limit for keyset pagination.
func (cfg KeysetConfig) ApplyToFind(find *options.FindOptions, sortField string) {
	find.SetSort(bson.D{
		{Key: sortField, Value: cfg.SortOrder},
		{Key: "_id", Value: cfg.SortOrder},
	}).SetLimit(LimitPlusOne())
}

// ApplyToFindModal is like ApplyToFind but uses ModalPageSize.
func (cfg KeysetConfig) ApplyToFindModal(find *options.FindOptions, sortField string) {
	find.SetSort(bson.D{
		{Key: sortField, Value: cfg.SortOrder},
		{Key: "_id", Value: cfg.SortOrder},
	}).SetLimit(ModalLimitPlusOne())
}

// KeysetWindow returns the cursor condition for the query filter.
// Returns nil if no cursor is set.
func (cfg KeysetConfig) KeysetWindow(sortField string) bson.M {
	if cfg.Cursor == nil {
		return nil
	}
	dir := "gt"
	if cfg.Direction == Backward {
		dir = "lt"
	}
	return wafflemongo.KeysetWindow(sortField, dir, cfg.Cursor.CI, cfg.Cursor.ID)
}

// Reverse reverses a slice in place. Use this after fetching results
// when paging backwards to restore the correct display order.
func Reverse[T any](rows []T) {
	for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
		rows[i], rows[j] = rows[j], rows[i]
	}
}

// BuildCursors creates prev/next cursor strings from the first and last elements.
// keyFn extracts the sort key from an element.
// idFn extracts the ObjectID from an element.
func BuildCursors[T any](rows []T, keyFn func(T) string, idFn func(T) primitive.ObjectID) (prev, next string) {
	if len(rows) == 0 {
		return "", ""
	}
	first := rows[0]
	last := rows[len(rows)-1]
	prev = wafflemongo.EncodeCursor(keyFn(first), idFn(first))
	next = wafflemongo.EncodeCursor(keyFn(last), idFn(last))
	return prev, next
}
