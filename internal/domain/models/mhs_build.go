// internal/domain/models/mhs_build.go
package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// MHSBuild tracks an individual unit build uploaded to S3.
// Each record represents one unit at a specific version.
type MHSBuild struct {
	ID              primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`
	UnitID          string             `bson:"unit_id" json:"unit_id"`                     // "unit1", "unit2", etc.
	Version         string             `bson:"version" json:"version"`                     // "2.2.3"
	BuildIdentifier string             `bson:"build_identifier" json:"build_identifier"`   // CI/CD build name, e.g. "CICDTesting/20260401-10923"
	Files           []MHSBuildFile     `bson:"files" json:"files"`                         // All files uploaded for this unit version
	TotalSize       int64              `bson:"total_size" json:"total_size"`               // Sum of file sizes in bytes
	DataFile        string             `bson:"data_file" json:"data_file"`                 // e.g. "unit1.data.unityweb"
	FrameworkFile   string             `bson:"framework_file" json:"framework_file"`       // e.g. "unit1.framework.js.unityweb"
	CodeFile        string             `bson:"code_file" json:"code_file"`                 // e.g. "unit1.wasm.unityweb"
	CreatedAt       time.Time          `bson:"created_at" json:"created_at"`
	CreatedByID     primitive.ObjectID `bson:"created_by_id" json:"created_by_id"`
	CreatedByName   string             `bson:"created_by_name" json:"created_by_name"`
}

// MHSBuildFile represents a single file in an MHS build.
type MHSBuildFile struct {
	Path string `bson:"path" json:"path"` // S3 key relative to prefix, e.g. "unit1/v2.2.3/Build/unit1.data.unityweb"
	Size int64  `bson:"size" json:"size"` // File size in bytes
}
