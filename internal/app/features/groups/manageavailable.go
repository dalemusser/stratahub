// internal/app/features/groups/manageavailable.go
package groups

import (
	"context"
	"maps"
	"strings"

	userstore "github.com/dalemusser/stratahub/internal/app/store/users"
	"github.com/dalemusser/stratahub/internal/app/system/paging"
	"github.com/dalemusser/stratahub/internal/app/system/search"
	"github.com/dalemusser/stratahub/internal/domain/models"
	wafflemongo "github.com/dalemusser/waffle/pantry/mongo"
	"github.com/dalemusser/waffle/pantry/text"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.uber.org/zap"
)

// fetchAvailablePaged returns a page of available members (not yet in group),
// with email-pivot logic for search.
func (h *Handler) fetchAvailablePaged(
	ctx context.Context,
	orgOID, groupOID primitive.ObjectID,
	qRaw, after, before string,
) (members []UserItem, shown int, total int64, nextCursor, prevCursor string, hasNext, hasPrev bool, err error) {

	db := h.DB
	usrStore := userstore.New(db)

	memberIDs, memberErr := h.fetchMemberIDs(ctx, db, groupOID, "member")
	if memberErr != nil {
		return nil, 0, 0, "", "", false, false, memberErr
	}

	filter := bson.M{
		"organization_id": orgOID,
		"role":            "member",
		"status":          "active",
	}
	if len(memberIDs) > 0 {
		filter["_id"] = bson.M{"$nin": memberIDs}
	}

	q := text.Fold(qRaw)
	status := "active"
	hasOrg := true

	emailPivot := search.EmailPivotOK(qRaw, status, hasOrg)

	if q != "" {
		sName := q
		hiName := sName + "\uffff"
		sEmail := strings.ToLower(strings.TrimSpace(qRaw))
		hiEmail := sEmail + "\uffff"

		if emailPivot {
			filter["$or"] = []bson.M{
				{"email": bson.M{"$gte": sEmail, "$lt": hiEmail}},
			}
		} else {
			filter["$or"] = []bson.M{
				{"full_name_ci": bson.M{"$gte": sName, "$lt": hiName}},
				{"email": bson.M{"$gte": sEmail, "$lt": hiEmail}},
			}
		}
	}

	// Count via store
	total, err = usrStore.Count(ctx, filter)
	if err != nil {
		h.Log.Error("database error counting available members", zap.Error(err))
		return nil, 0, 0, "", "", false, false, err
	}

	sortField := "full_name_ci"
	if emailPivot {
		sortField = "email"
	}

	// Configure keyset pagination
	findOpts := options.Find()
	cfg := paging.ConfigureKeyset(before, after)
	cfg.ApplyToFind(findOpts, sortField)

	// Apply cursor conditions (handle $or clause specially)
	if ks := cfg.KeysetWindow(sortField); ks != nil {
		if q != "" && filter["$or"] != nil {
			filter["$and"] = []bson.M{{"$or": filter["$or"].([]bson.M)}, ks}
			delete(filter, "$or")
		} else {
			maps.Copy(filter, ks)
		}
	}

	// Find via store
	rows, err := usrStore.Find(ctx, filter, findOpts)
	if err != nil {
		return nil, 0, 0, "", "", false, false, err
	}

	// Reverse if paging backwards
	if cfg.Direction == paging.Backward {
		paging.Reverse(rows)
	}

	// Apply pagination trimming
	page := paging.TrimPage(&rows, before, after)
	hasPrev, hasNext = page.HasPrev, page.HasNext

	members = make([]UserItem, 0, len(rows))
	for _, r := range rows {
		loginID := ""
		if r.LoginID != nil {
			loginID = *r.LoginID
		}
		members = append(members, UserItem{
			ID:       r.ID.Hex(),
			FullName: r.FullName,
			LoginID:  loginID,
		})
	}
	shown = len(members)
	if shown > 0 {
		first := rows[0]
		last := rows[shown-1]
		prevCursor = wafflemongo.EncodeCursor(first.FullNameCI, first.ID)
		nextCursor = wafflemongo.EncodeCursor(last.FullNameCI, last.ID)
	}

	return
}

// refreshAvailableMembers updates only the available members portion of page data.
// This avoids the N+1 pattern of rebuilding the entire page when only pagination
// adjustment is needed.
func (h *Handler) refreshAvailableMembers(
	ctx context.Context,
	data *ManagePageData,
	orgOID, groupOID primitive.ObjectID,
	q, after, before string,
) error {
	avail, shown, total, nextCur, prevCur, hasNext, hasPrev, err :=
		h.fetchAvailablePaged(ctx, orgOID, groupOID, q, after, before)
	if err != nil {
		return err
	}

	data.AvailableMembers = avail
	data.AvailableShown = shown
	data.AvailableTotal = total
	data.NextCursor = nextCur
	data.PrevCursor = prevCur
	data.HasNext = hasNext
	data.HasPrev = hasPrev
	data.CurrentAfter = after
	data.CurrentBefore = before
	data.Query = q

	return nil
}

// fetchAvailablePrevInclusive is used when paging backwards after removals
// and we need to pull a page that includes the anchor row.
func (h *Handler) fetchAvailablePrevInclusive(
	ctx context.Context,
	groupOID primitive.ObjectID,
	qRaw, after string,
) (members []UserItem, shown int, total int64, nextCursor, prevCursor string, hasNext, hasPrev bool, err error) {

	db := h.DB
	users := db.Collection("users")
	groupsColl := db.Collection("groups")

	var grp models.Group
	if err = groupsColl.FindOne(ctx, bson.M{"_id": groupOID}).Decode(&grp); err != nil {
		return
	}
	orgOID := grp.OrganizationID

	memberIDs, memberErr := h.fetchMemberIDs(ctx, db, groupOID, "member")
	if memberErr != nil {
		return nil, 0, 0, "", "", false, false, memberErr
	}

	filter := bson.M{
		"organization_id": orgOID,
		"role":            "member",
		"status":          "active",
	}
	if len(memberIDs) > 0 {
		filter["_id"] = bson.M{"$nin": memberIDs}
	}

	if cnt, cntErr := users.CountDocuments(ctx, filter); cntErr != nil {
		h.Log.Error("database error counting available members", zap.Error(cntErr))
		return nil, 0, 0, "", "", false, false, cntErr
	} else {
		total = cnt
	}

	q := text.Fold(qRaw)
	if q != "" {
		high := q + "\uffff"
		filter["full_name_ci"] = bson.M{"$gte": q, "$lt": high}
	}

	anchor, ok := wafflemongo.DecodeCursor(after)
	if !ok {
		cur, e := users.Find(ctx, filter,
			options.Find().SetSort(bson.D{{Key: "full_name_ci", Value: 1}, {Key: "_id", Value: 1}}).
				SetLimit(int64(paging.PageSize)))
		if e != nil {
			err = e
			return
		}
		defer cur.Close(ctx)

		var rows []struct {
			ID         primitive.ObjectID `bson:"_id"`
			FullName   string             `bson:"full_name"`
			LoginID    string             `bson:"login_id"`
			FullNameCI string             `bson:"full_name_ci"`
		}
		if err = cur.All(ctx, &rows); err != nil {
			return
		}

		members = make([]UserItem, 0, len(rows))
		for _, r := range rows {
			members = append(members, UserItem{
				ID:       r.ID.Hex(),
				FullName: r.FullName,
				LoginID:  r.LoginID,
			})
		}
		shown = len(members)
		if shown > 0 {
			prevCursor = wafflemongo.EncodeCursor(rows[0].FullNameCI, rows[0].ID)
			nextCursor = wafflemongo.EncodeCursor(rows[shown-1].FullNameCI, rows[shown-1].ID)

			last := rows[shown-1]
			fwd := bson.M{
				"organization_id": orgOID,
				"role":            "member",
				"$or": []bson.M{
					{"full_name_ci": bson.M{"$gt": last.FullNameCI}},
					{"full_name_ci": last.FullNameCI, "_id": bson.M{"$gt": last.ID}},
				},
			}
			if len(memberIDs) > 0 {
				fwd["_id"] = bson.M{"$nin": memberIDs}
			}
			fc, _ := users.Find(ctx, fwd,
				options.Find().SetSort(bson.D{{Key: "full_name_ci", Value: 1}, {Key: "_id", Value: 1}}).
					SetLimit(1))
			defer fc.Close(ctx)
			hasNext = fc.Next(ctx)
		}
		hasPrev = false
		return
	}

	// Backwards from anchor (inclusive).
	filter["$or"] = []bson.M{
		{"full_name_ci": bson.M{"$lt": anchor.CI}},
		{"full_name_ci": anchor.CI, "_id": bson.M{"$lte": anchor.ID}},
	}

	cur, e := users.Find(ctx, filter,
		options.Find().SetSort(bson.D{{Key: "full_name_ci", Value: -1}, {Key: "_id", Value: -1}}).
			SetLimit(paging.LimitPlusOne()))
	if e != nil {
		err = e
		return
	}
	defer cur.Close(ctx)

	var rowsDesc []struct {
		ID         primitive.ObjectID `bson:"_id"`
		FullName   string             `bson:"full_name"`
		LoginID    string             `bson:"login_id"`
		FullNameCI string             `bson:"full_name_ci"`
	}
	if err = cur.All(ctx, &rowsDesc); err != nil {
		return
	}

	for i, j := 0, len(rowsDesc)-1; i < j; i, j = i+1, j-1 {
		rowsDesc[i], rowsDesc[j] = rowsDesc[j], rowsDesc[i]
	}

	if len(rowsDesc) > paging.PageSize {
		rowsDesc = rowsDesc[1:]
		hasPrev = true
	} else {
		hasPrev = false
	}

	members = make([]UserItem, 0, len(rowsDesc))
	for _, r := range rowsDesc {
		members = append(members, UserItem{
			ID:       r.ID.Hex(),
			FullName: r.FullName,
			LoginID:  r.LoginID,
		})
	}
	shown = len(members)
	if shown > 0 {
		first := rowsDesc[0]
		last := rowsDesc[shown-1]
		prevCursor = wafflemongo.EncodeCursor(first.FullNameCI, first.ID)
		nextCursor = wafflemongo.EncodeCursor(last.FullNameCI, last.ID)

		fwd := bson.M{
			"organization_id": orgOID,
			"role":            "member",
			"$or": []bson.M{
				{"full_name_ci": bson.M{"$gt": last.FullNameCI}},
				{"full_name_ci": last.FullNameCI, "_id": bson.M{"$gt": last.ID}},
			},
		}
		if len(memberIDs) > 0 {
			fwd["_id"] = bson.M{"$nin": memberIDs}
		}
		fc, _ := users.Find(ctx, fwd,
			options.Find().SetSort(bson.D{{Key: "full_name_ci", Value: 1}, {Key: "_id", Value: 1}}).
				SetLimit(1))
		defer fc.Close(ctx)
		hasNext = fc.Next(ctx)
	} else {
		hasNext = false
	}

	return
}
