package missionhydrosci

import (
	"testing"

	"github.com/dalemusser/stratahub/internal/domain/models"
)

// The manifest lists a file's pieces when the record has the full count, not
// otherwise, and not when the kill switch is on.
func TestManifestListsPieces(t *testing.T) {
	big := int64(130 * 1024 * 1024)
	b := models.MHSBuild{Files: []models.MHSBuildFile{
		{Path: "unit4/v1/Build/unit4.data.unityweb", Size: big, Pieces: models.MHSPieceCount(big)},
		{Path: "unit4/v1/Build/unit4.wasm.unityweb", Size: 12 << 20},
		{Path: "unit4/v1/Build/other.data", Size: big, Pieces: 3}, // a stale count
	}}
	u := buildToManifestUnit("unit4", "Unit 4", "1", "", b)
	if f := u.Files[0]; f.Pieces != 17 || f.PieceBytes != models.MHSPieceBytes {
		t.Errorf("pieced file: %+v", f)
	}
	if u.Files[1].Pieces != 0 || u.Files[2].Pieces != 0 {
		t.Errorf("unpieced files: %+v %+v", u.Files[1], u.Files[2])
	}
	DownloadPieces = false
	defer func() { DownloadPieces = true }()
	if f := buildToManifestUnit("unit4", "Unit 4", "1", "", b).Files[0]; f.Pieces != 0 {
		t.Errorf("kill switch: %+v", f)
	}
}
