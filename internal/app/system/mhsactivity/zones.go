// Package mhsactivity computes what each Mission HydroSci student did on one
// calendar day (models.MHSActivityDay) from the load records, device rows,
// page views and game log the pages and the game leave behind. The same
// computation serves the activity page for an open day and the rollup job
// for a closed one (docs/mhs-activity/plan.md).
package mhsactivity

import (
	"context"
	"errors"
	"sort"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// DayFormat is how days are written ("2006-01-02", local to the organization).
const DayFormat = "2006-01-02"

// Zone groups the organizations of a workspace that share one time zone:
// their students' days begin and end at the same instant, so one computation
// covers them all.
type Zone struct {
	TZ       string // IANA name; "UTC" when no organization zone was usable
	Loc      *time.Location
	Fallback bool // the organizations had no usable zone
	OrgIDs   []primitive.ObjectID
}

// LocationFor loads an IANA zone, falling back to UTC (never the server's
// local zone) and reporting the fallback.
func LocationFor(tz string) (*time.Location, bool) {
	if tz == "" {
		return time.UTC, true
	}
	loc, err := time.LoadLocation(tz)
	if err != nil || loc == nil {
		return time.UTC, true
	}
	return loc, false
}

// ZonesFor groups the workspace's organizations by time zone. orgIDs nil
// means every organization; otherwise only those. Zones come back sorted by
// zone name, their organizations sorted by id.
func ZonesFor(ctx context.Context, db *mongo.Database, wsID primitive.ObjectID, orgIDs []primitive.ObjectID) ([]Zone, error) {
	filter := bson.M{"workspace_id": wsID}
	if orgIDs != nil {
		filter["_id"] = bson.M{"$in": orgIDs}
	}
	cur, err := db.Collection("organizations").Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	byZone := map[string]*Zone{}
	for cur.Next(ctx) {
		var o struct {
			ID       primitive.ObjectID `bson:"_id"`
			TimeZone string             `bson:"time_zone"`
		}
		if err := cur.Decode(&o); err != nil {
			return nil, err
		}
		loc, fallback := LocationFor(o.TimeZone)
		name := loc.String()
		z := byZone[name]
		if z == nil {
			z = &Zone{TZ: name, Loc: loc, Fallback: fallback}
			byZone[name] = z
		}
		z.OrgIDs = append(z.OrgIDs, o.ID)
	}
	if err := cur.Err(); err != nil {
		return nil, err
	}
	out := make([]Zone, 0, len(byZone))
	for _, z := range byZone {
		sort.Slice(z.OrgIDs, func(i, j int) bool { return z.OrgIDs[i].Hex() < z.OrgIDs[j].Hex() })
		out = append(out, *z)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TZ < out[j].TZ })
	return out, nil
}

// DayBounds returns [start, end) of a local day. Built with time.Date so a
// day across a daylight-saving change is 23 or 25 hours long, as it should be.
func DayBounds(day string, loc *time.Location) (start, end time.Time, err error) {
	t, err := time.ParseInLocation(DayFormat, day, loc)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	y, m, d := t.Date()
	start = time.Date(y, m, d, 0, 0, 0, 0, loc)
	end = time.Date(y, m, d+1, 0, 0, 0, 0, loc)
	return start, end, nil
}

// LocalDay is the day t falls on in loc.
func LocalDay(t time.Time, loc *time.Location) string {
	return t.In(loc).Format(DayFormat)
}

// Closed reports whether a local day ended more than closeAfter ago, so no
// page can still post a record carrying its entries (pages keep their log
// for six hours).
func Closed(day string, loc *time.Location, closeAfter time.Duration, now time.Time) bool {
	_, end, err := DayBounds(day, loc)
	if err != nil {
		return false
	}
	return !now.Before(end.Add(closeAfter))
}

// AddDays shifts a day string by n calendar days.
func AddDays(day string, n int) (string, error) {
	t, err := time.ParseInLocation(DayFormat, day, time.UTC)
	if err != nil {
		return "", err
	}
	return t.AddDate(0, 0, n).Format(DayFormat), nil
}

// DaysBetween lists the days from from to to inclusive.
func DaysBetween(from, to string) ([]string, error) {
	a, err := time.ParseInLocation(DayFormat, from, time.UTC)
	if err != nil {
		return nil, err
	}
	b, err := time.ParseInLocation(DayFormat, to, time.UTC)
	if err != nil {
		return nil, err
	}
	if b.Before(a) {
		return nil, errors.New("mhsactivity: the end day is before the start day")
	}
	var out []string
	for t := a; !t.After(b); t = t.AddDate(0, 0, 1) {
		out = append(out, t.Format(DayFormat))
	}
	return out, nil
}
