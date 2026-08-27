package viewers

import (
	"context"
	"encoding/csv"
	"io"

	"github.com/dalemusser/stratahub/internal/app/system/viewscope"
)

const (
	exportPageSize = 500
	exportRowCap   = 20000 // hard ceiling so a runaway filter cannot stream forever
)

// exportGeneric writes the viewer's rows as CSV by paging Query until the
// data is exhausted or the row cap is reached. Headers come from the
// column labels; cells contribute their Text (custom HTML cells export their
// Text too, so viewers should always set Text).
func exportGeneric(ctx context.Context, v Viewer, scope *viewscope.Scope, f Filters, w io.Writer) error {
	cw := csv.NewWriter(w)
	cols := v.Columns()
	header := make([]string, len(cols))
	for i, c := range cols {
		if c.Label != "" {
			header[i] = c.Label
		} else {
			header[i] = c.Key
		}
	}
	if err := cw.Write(header); err != nil {
		return err
	}

	var after *Cursor
	written := 0
	for {
		page, err := v.Query(ctx, scope, f, after, exportPageSize)
		if err != nil {
			return err
		}
		for _, row := range page.Rows {
			rec := make([]string, len(cols))
			for i := range cols {
				if i < len(row.Cells) {
					rec[i] = row.Cells[i].Text
				}
			}
			if err := cw.Write(rec); err != nil {
				return err
			}
			written++
			if written >= exportRowCap {
				cw.Flush()
				return cw.Error()
			}
		}
		if page.Next == nil || len(page.Rows) == 0 {
			break
		}
		after = page.Next
	}
	cw.Flush()
	return cw.Error()
}
