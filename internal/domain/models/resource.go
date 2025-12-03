package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Resource struct {
	ID      primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Title   string             `bson:"title" json:"title"`
	TitleCI string             `bson:"title_ci" json:"title_ci"` // lowercase, diacritics-stripped

	Subject   string `bson:"subject,omitempty" json:"subject,omitempty"`
	SubjectCI string `bson:"subject_ci,omitempty" json:"subject_ci,omitempty"`

	Type   string `bson:"type" json:"type"`     // e.g. "game", "survey", "tool"
	Status string `bson:"status" json:"status"` // "active" or "disabled"

	LaunchURL     string `bson:"launch_url,omitempty" json:"launch_url,omitempty"`
	ShowInLibrary bool   `bson:"show_in_library" json:"show_in_library"`

	Description         string `bson:"description,omitempty" json:"description,omitempty"`
	DefaultInstructions string `bson:"default_instructions,omitempty" json:"default_instructions,omitempty"`

	CreatedAt time.Time  `bson:"created_at" json:"created_at"`
	UpdatedAt *time.Time `bson:"updated_at,omitempty" json:"updated_at,omitempty"`

	CreatedByID   *primitive.ObjectID `bson:"created_by_id,omitempty" json:"created_by_id,omitempty"`
	CreatedByName string              `bson:"created_by_name,omitempty" json:"created_by_name,omitempty"`
	UpdatedByID   *primitive.ObjectID `bson:"updated_by_id,omitempty" json:"updated_by_id,omitempty"`
	UpdatedByName string              `bson:"updated_by_name,omitempty" json:"updated_by_name,omitempty"`
}
