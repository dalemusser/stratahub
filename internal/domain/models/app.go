package models

// AppDefinition describes an app that can be enabled/disabled per group.
type AppDefinition struct {
	ID          string // unique identifier (e.g., "mhs_units")
	Name        string // display name
	Description string // brief description for admin UI
	MenuIcon    string // emoji icon for sidebar menu
	MenuPath    string // URL path the app lives at
	MenuTitle   string // title attribute for the menu link
}

// AvailableApps is the registry of all apps that can be toggled per group.
var AvailableApps = []AppDefinition{
	{
		ID:          "mhs_units",
		Name:        "MHS Units",
		Description: "Mission HydroSci interactive science units",
		MenuIcon:    "\U0001F3AE", // game controller emoji
		MenuPath:    "/mhs/units",
		MenuTitle:   "Mission HydroSci",
	},
	{
		ID:          "missionhydrosci",
		Name:        "Mission HydroSci",
		Description: "Mission HydroSci single launch experience with auto-download and progress tracking",
		MenuIcon:    "\U0001F30A", // water wave emoji
		MenuPath:    "/missionhydrosci/units",
		MenuTitle:   "Mission HydroSci",
	},
}

// FindApp looks up an app definition by ID.
func FindApp(id string) (AppDefinition, bool) {
	for _, a := range AvailableApps {
		if a.ID == id {
			return a, true
		}
	}
	return AppDefinition{}, false
}
