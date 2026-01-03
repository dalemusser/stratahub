package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Resource represents content (games, surveys, tools, etc.) that can be
// assigned to Members via Groups.
//
// Resources support either a URL OR an uploaded file (mutually exclusive).
type Resource struct {
	ID          primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	WorkspaceID primitive.ObjectID `bson:"workspace_id" json:"workspace_id"` // Parent workspace
	Title       string             `bson:"title" json:"title"`
	TitleCI     string             `bson:"title_ci" json:"title_ci"` // lowercase, diacritics-stripped

	Subject   string `bson:"subject,omitempty" json:"subject,omitempty"`
	SubjectCI string `bson:"subject_ci,omitempty" json:"subject_ci,omitempty"`

	Type   string `bson:"type" json:"type"`     // e.g. "game", "survey", "tool"
	Status string `bson:"status" json:"status"` // "active" or "disabled"

	// Content source - mutually exclusive: either LaunchURL or file fields are set
	LaunchURL     string `bson:"launch_url,omitempty" json:"launch_url,omitempty"`
	ShowInLibrary bool   `bson:"show_in_library" json:"show_in_library"`

	// File storage fields - set when content is an uploaded file
	FilePath string `bson:"file_path,omitempty" json:"file_path,omitempty"` // Storage path (local or S3 key)
	FileName string `bson:"file_name,omitempty" json:"file_name,omitempty"` // Original filename
	FileSize int64  `bson:"file_size,omitempty" json:"file_size,omitempty"` // Size in bytes

	Description         string `bson:"description,omitempty" json:"description,omitempty"`
	DefaultInstructions string `bson:"default_instructions,omitempty" json:"default_instructions,omitempty"`

	CreatedAt time.Time  `bson:"created_at" json:"created_at"`
	UpdatedAt *time.Time `bson:"updated_at,omitempty" json:"updated_at,omitempty"`

	CreatedByID   *primitive.ObjectID `bson:"created_by_id,omitempty" json:"created_by_id,omitempty"`
	CreatedByName string              `bson:"created_by_name,omitempty" json:"created_by_name,omitempty"`
	UpdatedByID   *primitive.ObjectID `bson:"updated_by_id,omitempty" json:"updated_by_id,omitempty"`
	UpdatedByName string              `bson:"updated_by_name,omitempty" json:"updated_by_name,omitempty"`
}

// HasFile returns true if this resource has an uploaded file.
func (r *Resource) HasFile() bool {
	return r.FilePath != ""
}

// HasURL returns true if this resource has a launch URL.
func (r *Resource) HasURL() bool {
	return r.LaunchURL != ""
}
