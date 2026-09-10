package models

import (
	"testing"
	"time"

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

func TestStoppedResponding(t *testing.T) {
	now := time.Now().UTC()
	old := now.Add(-5 * time.Minute)
	game := &MHSDeviceTestHeartbeat{At: old, Phase: "game"}
	cases := []struct {
		name string
		run  MHSDeviceTest
		want bool
	}{
		{"no beats", MHSDeviceTest{}, false},
		{"recent beat", MHSDeviceTest{LastHeartbeatAt: &now, LastHeartbeat: &MHSDeviceTestHeartbeat{At: now, Phase: "game"}}, false},
		{"old game beat, no end", MHSDeviceTest{LastHeartbeatAt: &old, LastHeartbeat: game}, true},
		{"old beat but closed", MHSDeviceTest{LastHeartbeatAt: &old, LastHeartbeat: game, EndedAt: &old, EndReason: MHSDeviceTestEndClosed}, false},
		{"old beat but completed", MHSDeviceTest{LastHeartbeatAt: &old, LastHeartbeat: game, UnitCompletedAt: &old}, false},
		{"old download-phase beat", MHSDeviceTest{LastHeartbeatAt: &old, LastHeartbeat: &MHSDeviceTestHeartbeat{At: old, Phase: "download"}}, false},
	}
	for _, c := range cases {
		if got := c.run.StoppedResponding(now); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

func TestLaunchedSinceReset(t *testing.T) {
	base := time.Date(2026, 9, 10, 4, 0, 0, 0, time.UTC)
	at := func(m int) *time.Time { v := base.Add(time.Duration(m) * time.Minute); return &v }
	run := MHSDeviceTest{ReachedStage: MHSDeviceTestStageGameplay, LastLaunchAt: at(10), LastHeartbeatAt: at(12)}
	if !run.LaunchedSinceReset() {
		t.Fatal("never reset: a launched run must count as launched")
	}
	run.LastResetAt = at(20)
	if run.LaunchedSinceReset() {
		t.Fatal("after a reset with no later launch, the run must not count as launched")
	}
	run.LastLaunchAt = at(25)
	if !run.LaunchedSinceReset() {
		t.Fatal("a launch after the reset must count")
	}
	run.LastLaunchAt = at(10)
	run.LastHeartbeatAt = at(30)
	if !run.LaunchedSinceReset() {
		t.Fatal("a heartbeat after the reset must count as launched")
	}
}
