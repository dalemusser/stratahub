package paging

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestLimitPlusOne(t *testing.T) {
	want := int64(PageSize + 1)
	got := LimitPlusOne()
	if got != want {
		t.Errorf("LimitPlusOne() = %d, want %d", got, want)
	}
}

func TestTrimPage(t *testing.T) {
	tests := []struct {
		name       string
		rows       []int
		before     string
		after      string
		wantRows   []int
		wantResult Result
	}{
		{
			name:       "first page with no extra",
			rows:       []int{1, 2, 3},
			before:     "",
			after:      "",
			wantRows:   []int{1, 2, 3},
			wantResult: Result{HasPrev: false, HasNext: false},
		},
		{
			name:       "first page with extra (has next)",
			rows:       make([]int, PageSize+1),
			before:     "",
			after:      "",
			wantRows:   make([]int, PageSize),
			wantResult: Result{HasPrev: false, HasNext: true},
		},
		{
			name:       "forward page with extra",
			rows:       make([]int, PageSize+1),
			before:     "",
			after:      "cursor123",
			wantRows:   make([]int, PageSize),
			wantResult: Result{HasPrev: true, HasNext: true},
		},
		{
			name:       "forward page without extra",
			rows:       []int{1, 2, 3},
			before:     "",
			after:      "cursor123",
			wantRows:   []int{1, 2, 3},
			wantResult: Result{HasPrev: true, HasNext: false},
		},
		{
			name:       "backward page with extra",
			rows:       make([]int, PageSize+1),
			before:     "cursor123",
			after:      "",
			wantRows:   make([]int, PageSize),
			wantResult: Result{HasPrev: true, HasNext: true},
		},
		{
			name:       "backward page without extra",
			rows:       []int{1, 2, 3},
			before:     "cursor123",
			after:      "",
			wantRows:   []int{1, 2, 3},
			wantResult: Result{HasPrev: false, HasNext: true},
		},
		{
			name:       "empty rows",
			rows:       []int{},
			before:     "",
			after:      "",
			wantRows:   []int{},
			wantResult: Result{HasPrev: false, HasNext: false},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows := make([]int, len(tt.rows))
			copy(rows, tt.rows)

			got := TrimPage(&rows, tt.before, tt.after)

			if len(rows) != len(tt.wantRows) {
				t.Errorf("TrimPage() rows len = %d, want %d", len(rows), len(tt.wantRows))
			}
			if got.HasPrev != tt.wantResult.HasPrev {
				t.Errorf("TrimPage() HasPrev = %v, want %v", got.HasPrev, tt.wantResult.HasPrev)
			}
			if got.HasNext != tt.wantResult.HasNext {
				t.Errorf("TrimPage() HasNext = %v, want %v", got.HasNext, tt.wantResult.HasNext)
			}
		})
	}
}

func TestComputeRange(t *testing.T) {
	tests := []struct {
		name  string
		start int
		shown int
		want  Range
	}{
		{
			name:  "no results",
			start: 1,
			shown: 0,
			want:  Range{Start: 0, End: 0, PrevStart: 1, NextStart: 1},
		},
		{
			name:  "first page full",
			start: 1,
			shown: PageSize,
			want:  Range{Start: 1, End: PageSize, PrevStart: 1, NextStart: PageSize + 1},
		},
		{
			name:  "first page partial",
			start: 1,
			shown: 10,
			want:  Range{Start: 1, End: 10, PrevStart: 1, NextStart: 11},
		},
		{
			name:  "second page",
			start: PageSize + 1,
			shown: PageSize,
			want:  Range{Start: PageSize + 1, End: PageSize * 2, PrevStart: 1, NextStart: PageSize*2 + 1},
		},
		{
			name:  "middle page",
			start: 101,
			shown: 50,
			want:  Range{Start: 101, End: 150, PrevStart: 51, NextStart: 151},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ComputeRange(tt.start, tt.shown)
			if got != tt.want {
				t.Errorf("ComputeRange(%d, %d) = %+v, want %+v", tt.start, tt.shown, got, tt.want)
			}
		})
	}
}

func TestConfigureKeyset(t *testing.T) {
	tests := []struct {
		name      string
		before    string
		after     string
		wantDir   Direction
		wantOrder int
	}{
		{
			name:      "no cursors (first page)",
			before:    "",
			after:     "",
			wantDir:   Forward,
			wantOrder: 1,
		},
		{
			name:      "after cursor (forward)",
			before:    "",
			after:     "somecursor",
			wantDir:   Forward,
			wantOrder: 1,
		},
		{
			name:      "before cursor (backward)",
			before:    "somecursor",
			after:     "",
			wantDir:   Backward,
			wantOrder: -1,
		},
		{
			name:      "both cursors (before takes precedence)",
			before:    "beforecursor",
			after:     "aftercursor",
			wantDir:   Backward,
			wantOrder: -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ConfigureKeyset(tt.before, tt.after)
			if got.Direction != tt.wantDir {
				t.Errorf("ConfigureKeyset() Direction = %v, want %v", got.Direction, tt.wantDir)
			}
			if got.SortOrder != tt.wantOrder {
				t.Errorf("ConfigureKeyset() SortOrder = %v, want %v", got.SortOrder, tt.wantOrder)
			}
		})
	}
}

func TestReverse(t *testing.T) {
	tests := []struct {
		name  string
		input []int
		want  []int
	}{
		{"empty", []int{}, []int{}},
		{"single", []int{1}, []int{1}},
		{"two", []int{1, 2}, []int{2, 1}},
		{"three", []int{1, 2, 3}, []int{3, 2, 1}},
		{"four", []int{1, 2, 3, 4}, []int{4, 3, 2, 1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows := make([]int, len(tt.input))
			copy(rows, tt.input)
			Reverse(rows)
			for i, v := range rows {
				if v != tt.want[i] {
					t.Errorf("Reverse() got %v, want %v", rows, tt.want)
					break
				}
			}
		})
	}
}

func TestBuildCursors(t *testing.T) {
	t.Run("empty rows", func(t *testing.T) {
		type item struct {
			Key string
			ID  primitive.ObjectID
		}
		rows := []item{}
		prev, next := BuildCursors(rows,
			func(i item) string { return i.Key },
			func(i item) primitive.ObjectID { return i.ID },
		)
		if prev != "" || next != "" {
			t.Errorf("BuildCursors(empty) = (%q, %q), want (\"\", \"\")", prev, next)
		}
	})

	t.Run("single row", func(t *testing.T) {
		type item struct {
			Key string
			ID  primitive.ObjectID
		}
		id := primitive.NewObjectID()
		rows := []item{{Key: "test", ID: id}}
		prev, next := BuildCursors(rows,
			func(i item) string { return i.Key },
			func(i item) primitive.ObjectID { return i.ID },
		)
		// Both should be the same for a single row
		if prev == "" || next == "" {
			t.Errorf("BuildCursors(single) should return non-empty cursors")
		}
		if prev != next {
			t.Errorf("BuildCursors(single) prev and next should be equal for single row")
		}
	})

	t.Run("multiple rows", func(t *testing.T) {
		type item struct {
			Key string
			ID  primitive.ObjectID
		}
		id1 := primitive.NewObjectID()
		id2 := primitive.NewObjectID()
		rows := []item{
			{Key: "first", ID: id1},
			{Key: "last", ID: id2},
		}
		prev, next := BuildCursors(rows,
			func(i item) string { return i.Key },
			func(i item) primitive.ObjectID { return i.ID },
		)
		if prev == "" || next == "" {
			t.Errorf("BuildCursors(multiple) should return non-empty cursors")
		}
		if prev == next {
			t.Errorf("BuildCursors(multiple) prev and next should differ for multiple rows")
		}
	})
}
