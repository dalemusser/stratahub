package mhsbuilds

import (
	"bytes"
	"context"
	"testing"

	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/pantry/storage"
)

// A large file is stored as pieces of MHSPieceBytes (the last shorter) that
// join to the original bytes; small files get none; piecesPresent sees them.
func TestSplitIntoPieces(t *testing.T) {
	ctx := context.Background()
	st := storage.NewMemory(storage.MemoryConfig{})
	size := 2*models.MHSPieceBytes + 12345 + models.MHSPieceMinFileBytes
	big := make([]byte, size)
	for i := range big {
		big[i] = byte(i * 7)
	}
	files := []models.MHSBuildFile{
		{Path: "unit4/v1/Build/unit4.data.unityweb", Size: int64(size)},
		{Path: "unit4/v1/Build/unit4.loader.js", Size: 3},
	}
	data := map[string][]byte{files[0].Path: big, files[1].Path: []byte("abc")}
	if piecesPresent(ctx, st, files[0]) {
		t.Fatal("pieces present before the split")
	}
	if err := splitIntoPieces(ctx, st, files, data); err != nil {
		t.Fatal(err)
	}
	want := models.MHSPieceCount(int64(size))
	if files[0].Pieces != want || files[1].Pieces != 0 {
		t.Fatalf("pieces %d/%d, want %d/0", files[0].Pieces, files[1].Pieces, want)
	}
	var joined []byte
	for i := 0; i < want; i++ {
		b, err := st.GetBytes(ctx, models.MHSPiecePath(files[0].Path, i))
		if err != nil {
			t.Fatal(err)
		}
		if i < want-1 && len(b) != models.MHSPieceBytes {
			t.Fatalf("piece %d is %d bytes", i, len(b))
		}
		joined = append(joined, b...)
	}
	if !bytes.Equal(joined, big) {
		t.Fatal("the pieces do not join to the file")
	}
	if !piecesPresent(ctx, st, files[0]) {
		t.Fatal("pieces not seen after the split")
	}
	if got := models.MHSPiecePath("a/b.data", 7); got != "a/b.data.pieces/007" {
		t.Errorf("piece path %q", got)
	}
}
