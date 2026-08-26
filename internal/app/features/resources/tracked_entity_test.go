package resources_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/dalemusser/stratahub/internal/testutil"
	"go.mongodb.org/mongo-driver/bson"
)

// lookupTracked reads a resource's tracked_entity_id straight from the collection.
func lookupTracked(t *testing.T, fixtures *testutil.Fixtures, filter bson.M) (string, bool) {
	t.Helper()
	ctx, cancel := testutil.TestContext()
	defer cancel()
	var doc struct {
		TrackedEntityID string `bson:"tracked_entity_id"`
	}
	err := fixtures.DB().Collection("resources").FindOne(ctx, filter).Decode(&doc)
	if err != nil {
		return "", false
	}
	return doc.TrackedEntityID, true
}

// post submits a multipart form to the handler, swallowing the template
// panic that a validation-error re-render produces in unit tests (the
// template engine is not booted here).
func post(handler http.HandlerFunc, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	func() {
		defer func() { _ = recover() }()
		handler(rec, req)
	}()
	return rec
}

func TestHandleCreate_TrackedEntity(t *testing.T) {
	handler, fixtures := newTestAdminHandler(t)

	// Valid link persists.
	req, _ := createMultipartRequest(t, "/resources", map[string]string{
		"title": "Pre Survey Link", "launch_url": "https://surveys.example.com/pre", "type": "survey", "status": "active",
		"tracked_entity_id": "pre",
	})
	req = auth.WithTestUser(req, adminUser())
	if rec := post(handler.HandleCreate, req); rec.Code != http.StatusSeeOther {
		t.Fatalf("create: status %d, want 303", rec.Code)
	}
	if got, ok := lookupTracked(t, fixtures, bson.M{"title": "Pre Survey Link"}); !ok || got != "pre" {
		t.Errorf("tracked_entity_id: got %q (found=%v), want pre", got, ok)
	}

	// No link → field absent/empty.
	req, _ = createMultipartRequest(t, "/resources", map[string]string{
		"title": "Plain Game", "launch_url": "https://games.example.com/x", "type": "game", "status": "active",
	})
	req = auth.WithTestUser(req, adminUser())
	if rec := post(handler.HandleCreate, req); rec.Code != http.StatusSeeOther {
		t.Fatalf("create plain: status %d, want 303", rec.Code)
	}
	if got, ok := lookupTracked(t, fixtures, bson.M{"title": "Plain Game"}); !ok || got != "" {
		t.Errorf("plain resource tracked_entity_id: got %q (found=%v), want empty", got, ok)
	}

	// Unknown link id is rejected and nothing is created.
	req, _ = createMultipartRequest(t, "/resources", map[string]string{
		"title": "Bad Link", "launch_url": "https://surveys.example.com/bad", "type": "survey", "status": "active",
		"tracked_entity_id": "bogus",
	})
	req = auth.WithTestUser(req, adminUser())
	if rec := post(handler.HandleCreate, req); rec.Code == http.StatusSeeOther {
		t.Error("create with bogus tracked_entity_id should be rejected")
	}
	if _, ok := lookupTracked(t, fixtures, bson.M{"title": "Bad Link"}); ok {
		t.Error("rejected create still wrote a resource")
	}
}

func TestHandleEdit_TrackedEntity(t *testing.T) {
	handler, fixtures := newTestAdminHandler(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	res := fixtures.CreateResource(ctx, "Editable Survey", "https://surveys.example.com/e")
	id := res.ID.Hex()

	edit := func(tracked string) *httptest.ResponseRecorder {
		req, _ := createMultipartRequest(t, "/resources/"+id+"/edit", map[string]string{
			"title": "Editable Survey", "launch_url": "https://surveys.example.com/e", "type": "survey", "status": "active",
			"tracked_entity_id": tracked,
		})
		req = testutil.WithChiURLParam(req, "id", id)
		req = auth.WithTestUser(req, adminUser())
		return post(handler.HandleEdit, req)
	}

	// Set
	if rec := edit("mhs"); rec.Code != http.StatusSeeOther {
		t.Fatalf("edit set: status %d, want 303", rec.Code)
	}
	if got, _ := lookupTracked(t, fixtures, bson.M{"_id": res.ID}); got != "mhs" {
		t.Errorf("after set: %q, want mhs", got)
	}

	// Change
	if rec := edit("post"); rec.Code != http.StatusSeeOther {
		t.Fatalf("edit change: status %d, want 303", rec.Code)
	}
	if got, _ := lookupTracked(t, fixtures, bson.M{"_id": res.ID}); got != "post" {
		t.Errorf("after change: %q, want post", got)
	}

	// Invalid → rejected, unchanged
	if rec := edit("bogus"); rec.Code == http.StatusSeeOther {
		t.Error("edit with bogus tracked_entity_id should be rejected")
	}
	if got, _ := lookupTracked(t, fixtures, bson.M{"_id": res.ID}); got != "post" {
		t.Errorf("after rejected edit: %q, want post (unchanged)", got)
	}

	// Clear
	if rec := edit(""); rec.Code != http.StatusSeeOther {
		t.Fatalf("edit clear: status %d, want 303", rec.Code)
	}
	if got, _ := lookupTracked(t, fixtures, bson.M{"_id": res.ID}); got != "" {
		t.Errorf("after clear: %q, want empty", got)
	}

	// A stale id already stored (survey removed from config) survives an
	// unrelated edit that round-trips it.
	if _, err := fixtures.DB().Collection("resources").UpdateByID(ctx, res.ID, bson.M{"$set": bson.M{"tracked_entity_id": "retired"}}); err != nil {
		t.Fatal(err)
	}
	if rec := edit("retired"); rec.Code != http.StatusSeeOther {
		t.Fatalf("edit with stale id round-tripped: status %d, want 303", rec.Code)
	}
	if got, _ := lookupTracked(t, fixtures, bson.M{"_id": res.ID}); got != "retired" {
		t.Errorf("stale id: %q, want retired", got)
	}
}
