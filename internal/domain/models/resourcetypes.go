// internal/domain/models/resourcetypes.go
package models

// Canonical resource type identifiers.
//
// These values are stored in the database in the Resource.Type field and are
// used throughout the application as stable, language-agnostic keys.
// Human-facing labels for these types should be provided via i18n.
const (
	ResourceTypeAnimation    = "animation"
	ResourceTypeAnnouncement = "announcement"
	ResourceTypeArticle      = "article"
	ResourceTypeAssignment   = "assignment"
	ResourceTypeAudio        = "audio"
	ResourceTypeDataset      = "dataset"
	ResourceTypeDiscussion   = "discussion"
	ResourceTypeDocument     = "document"
	ResourceTypeExercise     = "exercise"
	ResourceTypeFAQ          = "faq"
	ResourceTypeGame         = "game"
	ResourceTypeLab          = "lab"
	ResourceTypeLesson       = "lesson"
	ResourceTypePresentation = "presentation"
	ResourceTypeProject      = "project"
	ResourceTypeQuiz         = "quiz"
	ResourceTypeReference    = "reference"
	ResourceTypeResource     = "resource" // generic catch-all; default
	ResourceTypeSchedule     = "schedule"
	ResourceTypeSimulation   = "simulation"
	ResourceTypeSurvey       = "survey"
	ResourceTypeTest         = "test"
	ResourceTypeTutorial     = "tutorial"
	ResourceTypeVideo        = "video"
	ResourceTypeWebsite      = "website"
)

// ResourceTypes is the full set of allowed resource type identifiers.
//
// This slice should be treated as the single source of truth for validation
// and schema enums. Any new type must be added here to be considered valid.
var ResourceTypes = []string{
	ResourceTypeAnimation,
	ResourceTypeAnnouncement,
	ResourceTypeArticle,
	ResourceTypeAssignment,
	ResourceTypeAudio,
	ResourceTypeDataset,
	ResourceTypeDiscussion,
	ResourceTypeDocument,
	ResourceTypeExercise,
	ResourceTypeFAQ,
	ResourceTypeGame,
	ResourceTypeLab,
	ResourceTypeLesson,
	ResourceTypePresentation,
	ResourceTypeProject,
	ResourceTypeQuiz,
	ResourceTypeReference,
	ResourceTypeResource,
	ResourceTypeSchedule,
	ResourceTypeSimulation,
	ResourceTypeSurvey,
	ResourceTypeTest,
	ResourceTypeTutorial,
	ResourceTypeVideo,
	ResourceTypeWebsite,
}

// DefaultResourceType is used when no specific type is provided.
const DefaultResourceType = ResourceTypeResource
