package models

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestNewMHSDeviceTestUserID(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		id, err := NewMHSDeviceTestUserID()
		if err != nil {
			t.Fatalf("mint: %v", err)
		}
		if len(id) != 24 || !IsMHSDeviceTestUserID(id) {
			t.Fatalf("bad id %q", id)
		}
		if _, err := primitive.ObjectIDFromHex(id); err != nil {
			t.Fatalf("id %q is not a valid ObjectID hex: %v", id, err)
		}
		if seen[id] {
			t.Fatalf("duplicate id %q", id)
		}
		seen[id] = true
	}
}

func TestIsMHSDeviceTestUserID(t *testing.T) {
	real := primitive.NewObjectID().Hex()
	if IsMHSDeviceTestUserID(real) {
		t.Fatalf("a fresh ObjectID %q must not look like a test id", real)
	}
	for _, bad := range []string{"", "ffffffff", "FFFFFFFF0123456789abcdef", "ffffffff0123456789abcde", "000000000000000000000001"} {
		if IsMHSDeviceTestUserID(bad) {
			t.Fatalf("%q must not be a test id", bad)
		}
	}
	if !IsMHSDeviceTestUserID("ffffffff0123456789abcdef") {
		t.Fatalf("marked id must be recognized")
	}
}
