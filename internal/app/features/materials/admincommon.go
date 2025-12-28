// internal/app/features/materials/admincommon.go
package materials

import (
	"html/template"
	"net/http"

	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/pantry/templates"
)

// renderNewForm populates the common chrome for the New Material page and
// renders the new form. Callers pass in a partially-filled materialFormVM
// (for example, to echo back user input on validation errors) and an
// optional error message.
func (h *AdminHandler) renderNewForm(w http.ResponseWriter, r *http.Request, vm materialFormVM, errMsg string) {
	vm.BaseVM = viewdata.NewBaseVM(r, h.DB, "New Material", "/materials")

	// Populate material type options for the select menu.
	vm.TypeOptions = materialTypeOptions()

	// Default type and status on initial GET.
	if vm.Type == "" {
		vm.Type = models.DefaultMaterialType
	}
	if vm.Status == "" {
		vm.Status = "active"
	}

	if errMsg != "" {
		vm.Error = template.HTML(errMsg)
	}

	// The admin "new" template is defined as "material_new".
	templates.Render(w, r, "material_new", vm)
}

// renderEditForm populates the common chrome for the Edit Material page and
// renders the edit form. Callers supply the current form VM plus an optional
// error message to display above the form.
func (h *AdminHandler) renderEditForm(w http.ResponseWriter, r *http.Request, vm materialFormVM, errMsg string) {
	// BackURL is where the simple "Back" link should go.
	backURL := "/materials"
	if vm.BackURL != "" {
		backURL = vm.BackURL
	}

	vm.BaseVM = viewdata.NewBaseVM(r, h.DB, "Edit Material", backURL)

	// SubmitReturn is the post-edit redirect target; DeleteReturn is used
	// by the delete button. If either is empty, default them to the
	// materials list so templates can rely on non-empty values.
	if vm.SubmitReturn == "" {
		vm.SubmitReturn = "/materials"
	}
	if vm.DeleteReturn == "" {
		vm.DeleteReturn = "/materials"
	}

	// Populate material type options for the select menu.
	vm.TypeOptions = materialTypeOptions()

	if errMsg != "" {
		vm.Error = template.HTML(errMsg)
	}

	// The admin "edit" template is defined as "material_edit".
	templates.Render(w, r, "material_edit", vm)
}
