// internal/domain/models/materialtypes.go
package models

// Canonical material type identifiers.
//
// These values are stored in the database in the Material.Type field and are
// used throughout the application as stable, language-agnostic keys.
// Human-facing labels for these types should be provided via i18n.
//
// Materials use the same type taxonomy as Resources for consistency.
const (
	MaterialTypeAnimation    = "animation"
	MaterialTypeAnnouncement = "announcement"
	MaterialTypeArticle      = "article"
	MaterialTypeAssignment   = "assignment"
	MaterialTypeAudio        = "audio"
	MaterialTypeDataset      = "dataset"
	MaterialTypeDiscussion   = "discussion"
	MaterialTypeDocument     = "document"
	MaterialTypeExercise     = "exercise"
	MaterialTypeFAQ          = "faq"
	MaterialTypeGame         = "game"
	MaterialTypeLab          = "lab"
	MaterialTypeLesson       = "lesson"
	MaterialTypePresentation = "presentation"
	MaterialTypeProject      = "project"
	MaterialTypeQuiz         = "quiz"
	MaterialTypeReference    = "reference"
	MaterialTypeMaterial     = "material" // generic catch-all; default
	MaterialTypeSchedule     = "schedule"
	MaterialTypeSimulation   = "simulation"
	MaterialTypeSurvey       = "survey"
	MaterialTypeTest         = "test"
	MaterialTypeTutorial     = "tutorial"
	MaterialTypeVideo        = "video"
	MaterialTypeWebsite      = "website"
)

// MaterialTypes is the full set of allowed material type identifiers.
//
// This slice should be treated as the single source of truth for validation
// and schema enums. Any new type must be added here to be considered valid.
var MaterialTypes = []string{
	MaterialTypeAnimation,
	MaterialTypeAnnouncement,
	MaterialTypeArticle,
	MaterialTypeAssignment,
	MaterialTypeAudio,
	MaterialTypeDataset,
	MaterialTypeDiscussion,
	MaterialTypeDocument,
	MaterialTypeExercise,
	MaterialTypeFAQ,
	MaterialTypeGame,
	MaterialTypeLab,
	MaterialTypeLesson,
	MaterialTypePresentation,
	MaterialTypeProject,
	MaterialTypeQuiz,
	MaterialTypeReference,
	MaterialTypeMaterial,
	MaterialTypeSchedule,
	MaterialTypeSimulation,
	MaterialTypeSurvey,
	MaterialTypeTest,
	MaterialTypeTutorial,
	MaterialTypeVideo,
	MaterialTypeWebsite,
}

// DefaultMaterialType is used when no specific type is provided.
const DefaultMaterialType = MaterialTypeMaterial
