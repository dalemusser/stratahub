// internal/app/features/mhsbuilds/pieces.go
package mhsbuilds

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/pantry/storage"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.uber.org/zap"
)

// Pieces (plan item B.7, docs/mission-hydrosci/mhs-resolution-plan-111426.md).
// Some school web filters hold the last megabytes of every large object they
// scan, whether it is asked for whole or as a range, so a unit's .data file
// never finishes downloading there (School T, November 2026), while objects
// of about 12 MB pass. Every build file of models.MHSPieceMinFileBytes or
// more is therefore also stored as pieces of models.MHSPieceBytes beside the
// whole file; the manifest lists the count and the service worker fetches
// the pieces and assembles the file. The whole file stays for everything
// else (older pages, the content route, the ceremony).

const piecesContentType = "application/octet-stream"

// putPieces reads size bytes from r and stores them as pieces of path.
// It holds one piece in memory at a time.
func putPieces(ctx context.Context, st storage.Store, path string, r io.Reader, size int64) (int, error) {
	buf := make([]byte, models.MHSPieceBytes)
	var total int64
	n := 0
	for {
		got, err := io.ReadFull(r, buf)
		if got > 0 {
			if perr := st.PutBytes(ctx, models.MHSPiecePath(path, n), buf[:got], &storage.PutOptions{ContentType: piecesContentType}); perr != nil {
				return 0, fmt.Errorf("store piece %d of %s: %w", n, path, perr)
			}
			n++
			total += int64(got)
		}
		if err == io.EOF || errors.Is(err, io.ErrUnexpectedEOF) {
			break
		}
		if err != nil {
			return 0, fmt.Errorf("read %s for pieces: %w", path, err)
		}
	}
	if total != size {
		return 0, fmt.Errorf("pieces of %s: %d bytes read, %d expected", path, total, size)
	}
	if n != models.MHSPieceCount(size) {
		return 0, fmt.Errorf("pieces of %s: %d stored, %d expected", path, n, models.MHSPieceCount(size))
	}
	return n, nil
}

// piecesPresent reports whether the content server already holds every piece
// of the file (the first and the last are checked by size; pieces are only
// ever written in order by putPieces).
func piecesPresent(ctx context.Context, st storage.Store, f models.MHSBuildFile) bool {
	count := models.MHSPieceCount(f.Size)
	last := f.Size - int64(count-1)*models.MHSPieceBytes
	first, err := st.Head(ctx, models.MHSPiecePath(f.Path, 0))
	if err != nil || first == nil || (count > 1 && first.Size != models.MHSPieceBytes) {
		return false
	}
	li, err := st.Head(ctx, models.MHSPiecePath(f.Path, count-1))
	return err == nil && li != nil && li.Size == last
}

// splitIntoPieces stores pieces for every large file of files whose data is
// in memory (an upload), and sets their counts.
func splitIntoPieces(ctx context.Context, st storage.Store, files []models.MHSBuildFile, data map[string][]byte) error {
	for i := range files {
		f := &files[i]
		if f.Size < models.MHSPieceMinFileBytes {
			continue
		}
		b, ok := data[f.Path]
		if !ok {
			continue
		}
		n, err := putPieces(ctx, st, f.Path, bytes.NewReader(b), f.Size)
		if err != nil {
			return err
		}
		f.Pieces = n
	}
	return nil
}

// ensureBuildPieces makes sure every large file of a unit build has its
// pieces on the content server and its count in the record. Returns how
// many files it changed.
func (h *Handler) ensureBuildPieces(ctx context.Context, b models.MHSBuild) (int, error) {
	if h.MHSStorage == nil || b.IsCeremony() {
		return 0, nil
	}
	files := append([]models.MHSBuildFile(nil), b.Files...)
	changed := 0
	for i := range files {
		f := &files[i]
		if f.Size < models.MHSPieceMinFileBytes {
			continue
		}
		want := models.MHSPieceCount(f.Size)
		if f.Pieces == want {
			continue
		}
		if !piecesPresent(ctx, h.MHSStorage, *f) {
			rc, err := h.MHSStorage.Get(ctx, f.Path)
			if err != nil {
				return changed, fmt.Errorf("read %s: %w", f.Path, err)
			}
			_, err = putPieces(ctx, h.MHSStorage, f.Path, rc, f.Size)
			rc.Close()
			if err != nil {
				return changed, err
			}
		}
		f.Pieces = want
		changed++
	}
	if changed == 0 {
		return 0, nil
	}
	if err := h.BuildStore.UpdateFiles(ctx, b.UnitID, b.Version, files, b.TotalSize, b.DataFile, b.FrameworkFile, b.CodeFile); err != nil {
		return changed, fmt.Errorf("record pieces for %s v%s: %w", b.UnitID, b.Version, err)
	}
	return changed, nil
}

// collectionsInUse returns the collections students can be given: each
// workspace's active collection and every group's own collection.
func (h *Handler) collectionsInUse(ctx context.Context) ([]primitive.ObjectID, error) {
	seen := map[primitive.ObjectID]bool{}
	var ids []primitive.ObjectID
	add := func(id *primitive.ObjectID) {
		if id != nil && !id.IsZero() && !seen[*id] {
			seen[*id] = true
			ids = append(ids, *id)
		}
	}
	var ss []struct {
		ID *primitive.ObjectID `bson:"mhs_active_collection_id"`
	}
	cur, err := h.DB.Collection("site_settings").Find(ctx, bson.M{"mhs_active_collection_id": bson.M{"$exists": true, "$ne": nil}})
	if err != nil {
		return nil, err
	}
	if err := cur.All(ctx, &ss); err != nil {
		return nil, err
	}
	for _, s := range ss {
		add(s.ID)
	}
	var gs []struct {
		ID *primitive.ObjectID `bson:"mhs_collection_id"`
	}
	cur, err = h.DB.Collection("group_app_settings").Find(ctx, bson.M{"app_id": "missionhydrosci", "mhs_collection_id": bson.M{"$exists": true, "$ne": nil}})
	if err != nil {
		return nil, err
	}
	if err := cur.All(ctx, &gs); err != nil {
		return nil, err
	}
	for _, g := range gs {
		add(g.ID)
	}
	return ids, nil
}

// EnsurePiecesInUse gives every unit build of the collections in use its
// pieces. Safe to run again: files that have them are skipped.
func (h *Handler) EnsurePiecesInUse(ctx context.Context) error {
	ids, err := h.collectionsInUse(ctx)
	if err != nil {
		return err
	}
	done := map[string]bool{}
	builds, files := 0, 0
	for _, id := range ids {
		coll, err := h.CollectionStore.GetByID(ctx, id)
		if err != nil {
			continue // a deleted collection still named by a setting
		}
		for _, u := range coll.Units {
			key := u.UnitID + "|" + u.Version
			if done[key] {
				continue
			}
			done[key] = true
			b, err := h.BuildStore.GetByUnitVersion(ctx, u.UnitID, u.Version)
			if err != nil {
				continue
			}
			n, err := h.ensureBuildPieces(ctx, b)
			if err != nil {
				h.Log.Warn("mhs build pieces", zap.String("unit", u.UnitID), zap.String("version", u.Version), zap.Error(err))
				continue
			}
			if n > 0 {
				builds++
				files += n
				h.Log.Info("mhs build pieces stored", zap.String("unit", u.UnitID), zap.String("version", u.Version), zap.Int("files", n))
			}
		}
	}
	h.Log.Info("mhs build pieces checked", zap.Int("collections", len(ids)), zap.Int("builds", len(done)), zap.Int("builds_changed", builds), zap.Int("files_changed", files))
	return nil
}

var piecesJobOnce sync.Once

// StartPiecesJob runs EnsurePiecesInUse a minute after start and then every
// hour (a collection made active or given to a group gets its pieces within
// the hour; an upload stores them at once). Only the first call starts it.
func (h *Handler) StartPiecesJob() {
	piecesJobOnce.Do(func() {
		go func() {
			time.Sleep(time.Minute)
			for {
				ctx, cancel := context.WithTimeout(context.Background(), 50*time.Minute)
				if err := h.EnsurePiecesInUse(ctx); err != nil {
					h.Log.Warn("mhs build pieces job", zap.Error(err))
				}
				cancel()
				time.Sleep(time.Hour)
			}
		}()
	})
}
