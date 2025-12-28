// internal/app/features/resources/admincommon.go
package resources

import (
	"html/template"
	"net/http"

	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/pantry/templates"
)

// renderNewForm populates the common chrome for the New Resource page and
// renders the new form. Callers pass in a partially-filled resourceFormVM
// (for example, to echo back user input on validation errors) and an
// optional error message.
func (h *AdminHandler) renderNewForm(w http.ResponseWriter, r *http.Request, vm resourceFormVM, errMsg string) {
	vm.BaseVM = viewdata.NewBaseVM(r, h.DB, "New Resource", "/resources")

	// Populate resource type options for the select menu.
	vm.TypeOptions = resourceTypeOptions()

	// Default type, status, and visibility on initial GET.
	if vm.Type == "" {
		vm.Type = models.DefaultResourceType
	}
	if vm.Status == "" {
		vm.Status = "active"
	}
	if r.Method == http.MethodGet && !vm.ShowInLibrary {
		vm.ShowInLibrary = true
	}

	if errMsg != "" {
		vm.Error = template.HTML(errMsg)
	}

	// The admin "new" template is defined as "resource_new".
	templates.Render(w, r, "resource_new", vm)
}

// renderEditForm populates the common chrome for the Edit Resource page and
// renders the edit form. Callers supply the current form VM plus an optional
// error message to display above the form.
func (h *AdminHandler) renderEditForm(w http.ResponseWriter, r *http.Request, vm resourceFormVM, errMsg string) {
	vm.BaseVM = viewdata.NewBaseVM(r, h.DB, "Edit Resource", "/resources")

	// SubmitReturn is the post-edit redirect target; DeleteReturn is used
	// by the delete button. If either is empty, default them to the
	// resources list so templates can rely on non-empty values.
	if vm.SubmitReturn == "" {
		vm.SubmitReturn = "/resources"
	}
	if vm.DeleteReturn == "" {
		vm.DeleteReturn = "/resources"
	}

	// Populate resource type options for the select menu.
	vm.TypeOptions = resourceTypeOptions()

	if errMsg != "" {
		vm.Error = template.HTML(errMsg)
	}

	// The admin "edit" template is defined as "resource_edit".
	templates.Render(w, r, "resource_edit", vm)
}
