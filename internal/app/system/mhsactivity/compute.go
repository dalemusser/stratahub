package mhsactivity

import (
	"context"
	"sort"
	"time"

	"github.com/dalemusser/stratahub/internal/app/store/activity"
	"github.com/dalemusser/stratahub/internal/app/store/logdata"
	"github.com/dalemusser/stratahub/internal/app/store/mhsdevicestatus"
	"github.com/dalemusser/stratahub/internal/app/store/mhsdevicetests"
	"github.com/dalemusser/stratahub/internal/app/system/mhssteplog"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

// Game is the game id in the log service and the grader.
const Game = "mhs"

// UnitsPagePath is the page whose views mean "opened".
const UnitsPagePath = "/missionhydrosci/units"

// MinCloseAfter is the least a day may stay open after it ends: the pages
// keep their step log for six hours, so a record stored up to six hours
// after midnight can still carry the day's entries.
const MinCloseAfter = 6 * time.Hour

// Names of the optional sources, as recorded in MHSActivityDay.Missing.
const (
	MissingLogData      = "logdata"
	MissingGrader       = "grader"
	MissingDeviceStatus = "device_status"
	MissingPageViews    = "page_views"
)

// Deps are the databases a computation reads. LogDB and GradesDB may be
// nil; the result then says so (Partial, Missing) instead of failing.
type Deps struct {
	DB         *mongo.Database // stratahub
	LogDB      *mongo.Database // stratalog
	GradesDB   *mongo.Database // mhsgrader
	Log        *zap.Logger
	CloseAfter time.Duration // how long after its end a day stays open; raised to MinCloseAfter when lower
}

func (d Deps) closeAfter() time.Duration {
	if d.CloseAfter < MinCloseAfter {
		return MinCloseAfter
	}
	return d.CloseAfter
}

func (d Deps) logger() *zap.Logger {
	if d.Log == nil {
		return zap.NewNop()
	}
	return d.Log
}

// person is a user of one of the zone's organizations.
type person struct {
	ID     primitive.ObjectID
	Org    primitive.ObjectID
	Role   string // MHSActivityRoleMember | MHSActivityRoleStaff
	Groups []primitive.ObjectID
}

// ComputeDay builds the day documents for every student and staff member of
// the zone's organizations for one local day. The result is not stored; the
// page uses it as is and the rollup job persists it. Required sources
// (people, load records) fail the call; optional ones (device rows, page
// views, the game log, the grader) mark the documents Partial.
func ComputeDay(ctx context.Context, deps Deps, wsID primitive.ObjectID, day string, zone Zone) ([]models.MHSActivityDay, error) {
	start, end, err := DayBounds(day, zone.Loc)
	if err != nil {
		return nil, err
	}
	log := deps.logger()

	people, err := loadPeople(ctx, deps.DB, wsID, zone.OrgIDs)
	if err != nil {
		return nil, err
	}
	if len(people) == 0 {
		return nil, nil
	}
	peopleIDs := make([]primitive.ObjectID, 0, len(people))
	hexIDs := make([]string, 0, len(people))
	for id := range people {
		peopleIDs = append(peopleIDs, id)
		hexIDs = append(hexIDs, id.Hex())
	}
	sort.Slice(peopleIDs, func(i, j int) bool { return peopleIDs[i].Hex() < peopleIDs[j].Hex() })
	sort.Strings(hexIDs)

	b := newBuilder(wsID, day, zone, start, end, people)

	// Load records: stored during the day, or up to closeAfter after it (a
	// page's log carries the day's entries that long). Each entry lands on
	// the day it happened.
	tests := mhsdevicetests.New(deps.DB)
	recs, err := tests.ListMemberBetween(ctx, wsID, start, end.Add(deps.closeAfter()))
	if err != nil {
		return nil, err
	}
	var needSteps []primitive.ObjectID
	kept := recs[:0]
	for _, rec := range recs {
		if rec.UserID == nil {
			continue
		}
		p := people[*rec.UserID]
		if p == nil {
			continue
		}
		if rec.OrganizationID != nil && !zone.has(*rec.OrganizationID) && zone.has(p.Org) {
			// The record names another organization but the user is here now:
			// count it here (the user moved).
		} else if rec.OrganizationID != nil && !zone.has(*rec.OrganizationID) {
			continue
		}
		kept = append(kept, rec)
		if rec.Summary == nil || rec.Summary.V != models.MHSMemberSummaryVersion {
			needSteps = append(needSteps, rec.ID)
		}
	}
	recs = kept
	var steps map[primitive.ObjectID][]models.MHSDeviceTestStep
	if len(needSteps) > 0 {
		steps, err = tests.StepsByIDs(ctx, wsID, needSteps)
		if err != nil {
			return nil, err
		}
	}
	for _, rec := range recs {
		sum := rec.Summary
		if sum == nil || sum.V != models.MHSMemberSummaryVersion {
			s := mhssteplog.Summarize(steps[rec.ID], outcomeOf(rec), mhssteplog.Context{
				UnitID:   rec.UnitID,
				DeviceID: rec.DeviceID,
			})
			sum = &s
		}
		b.addRecord(rec, sum)
	}

	// Device rows: a row created in the day is a fresh browser profile; one
	// seen in the day is an opened units page.
	if devs, err := mhsdevicestatus.New(deps.DB).ListSeenSince(ctx, wsID, start); err != nil {
		log.Warn("mhs activity: device status unavailable", zap.Error(err))
		b.missing(MissingDeviceStatus)
	} else {
		for _, d := range devs {
			b.addDevice(d)
		}
	}

	// Units-page views.
	if views, err := activity.New(deps.DB).PageViewsByUsers(ctx, peopleIDs, UnitsPagePath, start, end); err != nil {
		log.Warn("mhs activity: page views unavailable", zap.Error(err))
		b.missing(MissingPageViews)
	} else {
		for id, v := range views {
			b.addPageViews(id, v)
		}
	}

	// The game log: played.
	if deps.LogDB == nil {
		b.missing(MissingLogData)
	} else if played, err := logdata.New(deps.LogDB).ActivityByUsers(ctx, Game, hexIDs, start, end); err != nil {
		log.Warn("mhs activity: game log unavailable", zap.Error(err))
		b.missing(MissingLogData)
	} else {
		b.logAvailable = true
		for hexID, ua := range played {
			if id, err := primitive.ObjectIDFromHex(hexID); err == nil {
				b.addLog(id, ua)
			}
		}
	}

	docs := b.finish(time.Now().UTC())

	// Current unit from the grader, for the students the day touched.
	if len(docs) > 0 {
		if deps.GradesDB == nil {
			b.missing(MissingGrader)
		} else if err := fillCurrentUnits(ctx, deps.GradesDB, docs); err != nil {
			log.Warn("mhs activity: grader unavailable", zap.Error(err))
			b.missing(MissingGrader)
		}
		for i := range docs {
			docs[i].Partial = len(b.missingList) > 0
			docs[i].Missing = append([]string(nil), b.missingList...)
		}
	}
	return docs, nil
}

func (z Zone) has(org primitive.ObjectID) bool {
	for _, id := range z.OrgIDs {
		if id == org {
			return true
		}
	}
	return false
}

func outcomeOf(rec models.MHSDeviceTest) string {
	if rec.Diagnostics == nil {
		return ""
	}
	if o, ok := rec.Diagnostics["outcome"].(string); ok {
		return o
	}
	return ""
}

// loadPeople returns the zone's users (students and staff) and their groups.
func loadPeople(ctx context.Context, db *mongo.Database, wsID primitive.ObjectID, orgIDs []primitive.ObjectID) (map[primitive.ObjectID]*person, error) {
	people := map[primitive.ObjectID]*person{}
	cur, err := db.Collection("users").Find(ctx,
		bson.M{"workspace_id": wsID, "organization_id": bson.M{"$in": orgIDs}},
		optionsProjection(bson.M{"_id": 1, "role": 1, "organization_id": 1}))
	if err != nil {
		return nil, err
	}
	for cur.Next(ctx) {
		var u struct {
			ID   primitive.ObjectID  `bson:"_id"`
			Role string              `bson:"role"`
			Org  *primitive.ObjectID `bson:"organization_id"`
		}
		if err := cur.Decode(&u); err != nil {
			cur.Close(ctx)
			return nil, err
		}
		if u.Org == nil {
			continue
		}
		role := models.MHSActivityRoleStaff
		if u.Role == "member" {
			role = models.MHSActivityRoleMember
		}
		people[u.ID] = &person{ID: u.ID, Org: *u.Org, Role: role}
	}
	err = cur.Err()
	cur.Close(ctx)
	if err != nil {
		return nil, err
	}
	if len(people) == 0 {
		return people, nil
	}

	mcur, err := db.Collection("group_memberships").Find(ctx,
		bson.M{"workspace_id": wsID, "org_id": bson.M{"$in": orgIDs}},
		optionsProjection(bson.M{"user_id": 1, "group_id": 1, "role": 1}))
	if err != nil {
		return nil, err
	}
	defer mcur.Close(ctx)
	for mcur.Next(ctx) {
		var m struct {
			UserID  primitive.ObjectID `bson:"user_id"`
			GroupID primitive.ObjectID `bson:"group_id"`
			Role    string             `bson:"role"`
		}
		if err := mcur.Decode(&m); err != nil {
			return nil, err
		}
		p := people[m.UserID]
		if p == nil {
			continue
		}
		// A student's groups are the ones they are a member of; a teacher's
		// are the ones they lead. Either way the row's role must match the
		// person's, so a teacher attached as "leader" to a group is never
		// read as one of its students.
		if (p.Role == models.MHSActivityRoleMember) != (m.Role == "member") {
			continue
		}
		p.Groups = append(p.Groups, m.GroupID)
	}
	if err := mcur.Err(); err != nil {
		return nil, err
	}
	for _, p := range people {
		sort.Slice(p.Groups, func(i, j int) bool { return p.Groups[i].Hex() < p.Groups[j].Hex() })
	}
	return people, nil
}

// fillCurrentUnits sets CurrentUnit from the grader for the students in docs.
func fillCurrentUnits(ctx context.Context, gradesDB *mongo.Database, docs []models.MHSActivityDay) error {
	hexIDs := make([]string, 0, len(docs))
	for _, d := range docs {
		hexIDs = append(hexIDs, d.UserID.Hex())
	}
	cur, err := gradesDB.Collection("progress_point_grades").Find(ctx,
		bson.M{"game": Game, "user_id": bson.M{"$in": hexIDs}},
		optionsProjection(bson.M{"user_id": 1, "currentUnit": 1}))
	if err != nil {
		return err
	}
	defer cur.Close(ctx)
	current := map[string]string{}
	for cur.Next(ctx) {
		var g struct {
			UserID      string `bson:"user_id"`
			CurrentUnit string `bson:"currentUnit"`
		}
		if err := cur.Decode(&g); err != nil {
			return err
		}
		current[g.UserID] = g.CurrentUnit
	}
	if err := cur.Err(); err != nil {
		return err
	}
	for i := range docs {
		docs[i].CurrentUnit = current[docs[i].UserID.Hex()]
	}
	return nil
}
