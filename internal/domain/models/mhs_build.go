// internal/domain/models/mhs_build.go
package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Build kinds. A unit build is a Unity WebGL unit (the default; older records
// carry no kind). The ceremony build is the end-of-game ceremony web bundle
// (mhs-gameplay-end), stored under the same prefix as the units at
// "end/vX.Y.Z/" and referenced by a collection's Ceremony field. It is not a
// unit: it has no Unity files, is never listed on the units page, and does not
// count toward completing the game (docs/mission-hydrosci/mhs-end-ceremony-plan.md D1).
const (
	MHSBuildKindUnit     = "unit"
	MHSBuildKindCeremony = "ceremony"

	// MHSCeremonyBuildID is the ceremony's id in unit_id, the top-level folder
	// name on the CDN, and the id in content URLs (/missionhydrosci/content/end/...).
	MHSCeremonyBuildID = "end"

	// MHSCeremonyEntryFile is the file a host page loads to run the ceremony
	// (mhs-gameplay-end/docs/embed-api.md); the S3 sync recognises a version
	// folder as a ceremony build by its presence.
	MHSCeremonyEntryFile = "lib/embed.js"
)

// MHSBuild tracks an individual build uploaded to S3: a unit at a specific
// version, or the ceremony at a specific version (Kind).
type MHSBuild struct {
	ID              primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`
	UnitID          string             `bson:"unit_id" json:"unit_id"`                           // "unit1", "unit2", etc.; "end" for the ceremony
	Kind            string             `bson:"kind,omitempty" json:"kind,omitempty"`             // MHSBuildKindUnit (default when empty) or MHSBuildKindCeremony
	EntryFile       string             `bson:"entry_file,omitempty" json:"entry_file,omitempty"` // ceremony only: the embed script, relative to the version folder
	Version         string             `bson:"version" json:"version"`                           // "2.2.3"
	BuildIdentifier string             `bson:"build_identifier" json:"build_identifier"`         // CI/CD build name, e.g. "CICDTesting/20260401-10923"
	Files           []MHSBuildFile     `bson:"files" json:"files"`                               // All files uploaded for this unit version
	TotalSize       int64              `bson:"total_size" json:"total_size"`                     // Sum of file sizes in bytes
	DataFile        string             `bson:"data_file" json:"data_file"`                       // e.g. "unit1.data.unityweb"
	FrameworkFile   string             `bson:"framework_file" json:"framework_file"`             // e.g. "unit1.framework.js.unityweb"
	CodeFile        string             `bson:"code_file" json:"code_file"`                       // e.g. "unit1.wasm.unityweb"
	CreatedAt       time.Time          `bson:"created_at" json:"created_at"`
	CreatedByID     primitive.ObjectID `bson:"created_by_id" json:"created_by_id"`
	CreatedByName   string             `bson:"created_by_name" json:"created_by_name"`
}

// IsCeremony reports whether the build is the end-of-game ceremony bundle.
func (b MHSBuild) IsCeremony() bool { return b.Kind == MHSBuildKindCeremony }

// MHSBuildFile represents a single file in an MHS build.
type MHSBuildFile struct {
	Path string `bson:"path" json:"path"` // S3 key relative to prefix, e.g. "unit1/v2.2.3/Build/unit1.data.unityweb"
	Size int64  `bson:"size" json:"size"` // File size in bytes
}
