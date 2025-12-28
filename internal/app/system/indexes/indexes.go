// internal/app/system/indexes/indexes.go
package indexes

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.uber.org/zap"
)

/*
EnsureAll is called at startup. Each ensure* function is idempotent.
We aggregate errors so any problem is visible and startup can fail fast.
*/
func EnsureAll(ctx context.Context, db *mongo.Database) error {
	var problems []string

	if err := ensureUsers(ctx, db); err != nil {
		problems = append(problems, "users: "+err.Error())
	}
	if err := ensureOrganizations(ctx, db); err != nil {
		problems = append(problems, "organizations: "+err.Error())
	}
	if err := ensureGroups(ctx, db); err != nil {
		problems = append(problems, "groups: "+err.Error())
	}
	if err := ensureGroupMemberships(ctx, db); err != nil {
		problems = append(problems, "group_memberships: "+err.Error())
	}
	if err := ensureResources(ctx, db); err != nil {
		problems = append(problems, "resources: "+err.Error())
	}
	if err := ensureGroupResourceAssignments(ctx, db); err != nil {
		problems = append(problems, "group_resource_assignments: "+err.Error())
	}
	if err := ensureMaterials(ctx, db); err != nil {
		problems = append(problems, "materials: "+err.Error())
	}
	if err := ensureMaterialAssignments(ctx, db); err != nil {
		problems = append(problems, "material_assignments: "+err.Error())
	}
	// dashboards typically read "recent activity" from login_records
	if err := ensureLoginRecords(ctx, db); err != nil {
		problems = append(problems, "login_records: "+err.Error())
	}
	if err := ensurePages(ctx, db); err != nil {
		problems = append(problems, "pages: "+err.Error())
	}

	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}

/* -------------------------------------------------------------------------- */
/* Core helper: reconcile a set of desired indexes for one collection         */
/* -------------------------------------------------------------------------- */

type existingIndex struct {
	Name   string `bson:"name"`
	Key    bson.D `bson:"key"`
	Unique *bool  `bson:"unique,omitempty"`
}

func keySig(keys bson.D) string {
	parts := make([]string, 0, len(keys))
	for _, kv := range keys {
		parts = append(parts, fmt.Sprintf("%s:%v", kv.Key, kv.Value))
	}
	return strings.Join(parts, ", ")
}

func sameBoolPtr(a, b *bool) bool {
	av := false
	bv := false
	if a != nil {
		av = *a
	}
	if b != nil {
		bv = *b
	}
	return av == bv
}

// Best-effort duplicate-detector (works cross-vendors)
func isDuplicateKeyErr(err error) bool {
	if err == nil {
		return false
	}
	var we mongo.WriteException
	if errors.As(err, &we) {
		for _, e := range we.WriteErrors {
			if e.Code == 11000 { // E11000 duplicate key error index
				return true
			}
		}
	}
	var ce mongo.CommandError
	if errors.As(err, &ce) && ce.Code == 11000 {
		return true
	}
	s := err.Error()
	return strings.Contains(s, "E11000") || strings.Contains(strings.ToLower(s), "duplicate key")
}

// Mongo/DocDB sometimes returns IndexOptionsConflict when an index with the
// same keys already exists under a different name (or options differ).
func isOptionsConflictErr(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "IndexOptionsConflict")
}

func ensureIndexSet(ctx context.Context, coll *mongo.Collection, models []mongo.IndexModel) error {
	var errs []string

	for _, m := range models {
		var desiredName string
		var desiredUnique *bool
		if m.Options != nil {
			if m.Options.Name != nil {
				desiredName = *m.Options.Name
			}
			if m.Options.Unique != nil {
				desiredUnique = m.Options.Unique
			}
		}
		desiredSig := keySig(m.Keys.(bson.D))

		start := time.Now()
		zap.L().Info("ensuring index",
			zap.String("collection", coll.Name()),
			zap.String("name", desiredName),
			zap.String("keys", desiredSig),
			zap.Bool("unique", desiredUnique != nil && *desiredUnique))

		// 1) Load existing indexes
		existing := map[string]existingIndex{} // sig -> index
		cur, err := coll.Indexes().List(ctx)
		if err == nil {
			defer cur.Close(ctx)
			for cur.Next(ctx) {
				var idx existingIndex
				if err := cur.Decode(&idx); err != nil {
					zap.L().Warn("failed to decode existing index",
						zap.String("collection", coll.Name()),
						zap.Error(err))
					continue
				}
				existing[keySig(idx.Key)] = idx
			}
		}

		if ex, ok := existing[desiredSig]; ok {
			// Same key pattern exists already.
			if sameBoolPtr(desiredUnique, ex.Unique) {
				// --- Name alignment: if the name differs, drop & recreate with the desired name.
				if desiredName != "" && ex.Name != desiredName {
					zap.L().Info("renaming index to align with desired name",
						zap.String("collection", coll.Name()),
						zap.String("from", ex.Name),
						zap.String("to", desiredName),
						zap.String("keys", desiredSig))

					if _, err := coll.Indexes().DropOne(ctx, ex.Name); err != nil {
						zap.L().Warn("drop existing index (rename) failed",
							zap.String("collection", coll.Name()),
							zap.String("name", ex.Name),
							zap.Error(err))
						errs = append(errs, fmt.Sprintf("%s(%s): rename drop failed: %v", coll.Name(), desiredName, err))
						continue
					}
					if _, err := coll.Indexes().CreateOne(ctx, m); err != nil {
						zap.L().Warn("create index (rename) failed",
							zap.String("collection", coll.Name()),
							zap.String("name", desiredName),
							zap.Error(err))
						errs = append(errs, fmt.Sprintf("%s(%s): rename create failed: %v", coll.Name(), desiredName, err))
						continue
					}
					zap.L().Info("index renamed",
						zap.String("collection", coll.Name()),
						zap.String("name", desiredName),
						zap.String("keys", desiredSig),
						zap.String("took", time.Since(start).String()))
					continue
				}

				// Names aligned (or we don't care) → reuse
				zap.L().Info("reusing existing index",
					zap.String("collection", coll.Name()),
					zap.String("name", ex.Name),
					zap.String("keys", desiredSig),
					zap.Bool("unique", ex.Unique != nil && *ex.Unique),
					zap.String("took", time.Since(start).String()))
				continue
			}

			// Options mismatch (e.g., upgrading to unique). Drop & recreate.
			if _, err := coll.Indexes().DropOne(ctx, ex.Name); err != nil {
				zap.L().Warn("drop existing index failed",
					zap.String("collection", coll.Name()),
					zap.String("name", ex.Name),
					zap.String("keys", desiredSig),
					zap.Error(err))
				errs = append(errs, fmt.Sprintf("%s(%s): drop failed: %v", coll.Name(), desiredName, err))
				continue
			}
			if _, err := coll.Indexes().CreateOne(ctx, m); err != nil {
				if isDuplicateKeyErr(err) && desiredUnique != nil && *desiredUnique {
					helper := ""
					if coll.Name() == "users" && strings.Contains(desiredSig, "email:1") {
						helper = " — duplicates exist on users.email. Example finder:\n" +
							`db.users.aggregate([{ $group: { _id: "$email", n: { $sum: 1 } } }, { $match: { n: { $gt: 1 } } }])`
					}
					errs = append(errs, fmt.Sprintf("%s(%s): cannot create unique index (duplicates present)%s", coll.Name(), desiredName, helper))
				} else {
					errs = append(errs, fmt.Sprintf("%s(%s): %v", coll.Name(), desiredName, err))
				}
				continue
			}
			zap.L().Info("index dropped and recreated",
				zap.String("collection", coll.Name()),
				zap.String("name", desiredName),
				zap.String("keys", desiredSig),
				zap.Bool("unique", desiredUnique != nil && *desiredUnique),
				zap.String("took", time.Since(start).String()))
			continue
		}

		// 2) No existing index with the same keys: create it.
		if created, err := coll.Indexes().CreateOne(ctx, m); err != nil {
			if isOptionsConflictErr(err) {
				cur2, e2 := coll.Indexes().List(ctx)
				if e2 == nil {
					var match *existingIndex
					for cur2.Next(ctx) {
						var idx existingIndex
						if err := cur2.Decode(&idx); err != nil {
							zap.L().Warn("failed to decode existing index (post-conflict)",
								zap.String("collection", coll.Name()),
								zap.Error(err))
							continue
						}
						if keySig(idx.Key) == desiredSig {
							match = &idx
							break
						}
					}
					cur2.Close(ctx)
					if match != nil {
						if sameBoolPtr(desiredUnique, match.Unique) {
							// Optional: we could perform the same rename logic here, but it's
							// rare to hit this branch immediately after CreateOne().
							zap.L().Info("reusing existing index (post-conflict)",
								zap.String("collection", coll.Name()),
								zap.String("name", match.Name),
								zap.String("keys", desiredSig),
								zap.Bool("unique", match.Unique != nil && *match.Unique),
								zap.String("took", time.Since(start).String()))
							continue
						}
						if _, dropErr := coll.Indexes().DropOne(ctx, match.Name); dropErr != nil {
							zap.L().Warn("failed to drop conflicting index",
								zap.String("collection", coll.Name()),
								zap.String("name", match.Name),
								zap.Error(dropErr))
						}
						if _, e3 := coll.Indexes().CreateOne(ctx, m); e3 != nil {
							if isDuplicateKeyErr(e3) && desiredUnique != nil && *desiredUnique {
								helper := ""
								if coll.Name() == "users" && strings.Contains(desiredSig, "email:1") {
									helper = " — duplicates exist on users.email. Example finder:\n" +
										`db.users.aggregate([{ $group: { _id: "$email", n: { $sum: 1 } } }, { $match: { n: { $gt: 1 } } }])`
								}
								errs = append(errs, fmt.Sprintf("%s(%s): cannot create unique index (duplicates present)%s", coll.Name(), desiredName, helper))
							} else {
								errs = append(errs, fmt.Sprintf("%s(%s): %v", coll.Name(), desiredName, e3))
							}
							continue
						}
						zap.L().Info("index dropped and recreated (post-conflict)",
							zap.String("collection", coll.Name()),
							zap.String("name", desiredName),
							zap.String("keys", desiredSig),
							zap.Bool("unique", desiredUnique != nil && *desiredUnique),
							zap.String("took", time.Since(start).String()))
						continue
					}
				}

				zap.L().Warn("index ensure failed",
					zap.String("collection", coll.Name()),
					zap.String("name", desiredName),
					zap.String("keys", desiredSig),
					zap.Bool("unique", desiredUnique != nil && *desiredUnique),
					zap.String("took", time.Since(start).String()),
					zap.Error(err))
				errs = append(errs, fmt.Sprintf("%s(%s): %v", coll.Name(), desiredName, err))
				continue
			}

			zap.L().Warn("index ensure failed",
				zap.String("collection", coll.Name()),
				zap.String("name", desiredName),
				zap.String("keys", desiredSig),
				zap.Bool("unique", desiredUnique != nil && *desiredUnique),
				zap.String("took", time.Since(start).String()),
				zap.Error(err))
			errs = append(errs, fmt.Sprintf("%s(%s): %v", coll.Name(), desiredName, err))
			continue
		} else {
			zap.L().Info("index ensured",
				zap.String("collection", coll.Name()),
				zap.String("name", desiredName),
				zap.String("created_name", created),
				zap.String("keys", desiredSig),
				zap.Bool("unique", desiredUnique != nil && *desiredUnique),
				zap.String("took", time.Since(start).String()))
		}
	}

	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

/* -------------------------------------------------------------------------- */
/* Collection-specific index sets                                              */
/* -------------------------------------------------------------------------- */

func ensureUsers(ctx context.Context, db *mongo.Database) error {
	c := db.Collection("users")
	return ensureIndexSet(ctx, c, []mongo.IndexModel{
		// 1) Email must be unique across all users (global, cross‑org)
		{
			Keys:    bson.D{{Key: "email", Value: 1}},
			Options: options.Index().SetUnique(true).SetName("uniq_users_email"),
		},

		// 2) Members lists (org-scoped): covers both with and without status filter.
		//    Field order {role, org, status, name, _id} allows:
		//    - {role, org} queries to use the prefix, then scan status values
		//    - {role, org, status} queries to use the full prefix efficiently
		//    This consolidates what was previously two separate indexes.
		{
			Keys: bson.D{
				{Key: "role", Value: 1},
				{Key: "organization_id", Value: 1},
				{Key: "status", Value: 1},
				{Key: "full_name_ci", Value: 1},
				{Key: "_id", Value: 1},
			},
			Options: options.Index().SetName("idx_users_role_org_status_fullnameci_id"),
		},

		// 3) System-users lists (no org filter): admin/analyst queries.
		//    Queries without status filter use {role} prefix and scan status values.
		//    Queries with status filter use full prefix efficiently.
		//    System user counts are typically small, so the scan is acceptable.
		{
			Keys: bson.D{
				{Key: "role", Value: 1},
				{Key: "status", Value: 1},
				{Key: "full_name_ci", Value: 1},
				{Key: "_id", Value: 1},
			},
			Options: options.Index().SetName("idx_users_role_status_fullnameci_id"),
		},

		// 4) Email search path when you pivot sort to email (members screens)
		{
			Keys: bson.D{
				{Key: "role", Value: 1},
				{Key: "organization_id", Value: 1},
				{Key: "status", Value: 1},
				{Key: "email", Value: 1},
				{Key: "_id", Value: 1},
			},
			Options: options.Index().SetName("idx_users_role_org_status_email_id"),
		},

		// 5) Handy single-field lookup
		{
			Keys:    bson.D{{Key: "organization_id", Value: 1}},
			Options: options.Index().SetName("idx_users_org"),
		},

		// 6) Fast per‑org counts by role (used in side panes)
		{
			Keys:    bson.D{{Key: "role", Value: 1}, {Key: "organization_id", Value: 1}},
			Options: options.Index().SetName("idx_users_role_org"),
		},
	})
}

func ensureOrganizations(ctx context.Context, db *mongo.Database) error {
	c := db.Collection("organizations")
	return ensureIndexSet(ctx, c, []mongo.IndexModel{
		// Enforce global uniqueness of organization names (case/diacritics folded).
		{
			Keys:    bson.D{{Key: "name_ci", Value: 1}},
			Options: options.Index().SetUnique(true).SetName("uniq_orgs_nameci"),
		},

		// Name prefix search + stable sort
		{
			Keys:    bson.D{{Key: "name_ci", Value: 1}, {Key: "_id", Value: 1}},
			Options: options.Index().SetName("idx_orgs_nameci__id"),
		},
		// Filter by status, then name_ci sort
		{
			Keys:    bson.D{{Key: "status", Value: 1}, {Key: "name_ci", Value: 1}, {Key: "_id", Value: 1}},
			Options: options.Index().SetName("idx_orgs_status_nameci__id"),
		},
		// Optional city/state prefix searches
		{
			Keys:    bson.D{{Key: "city_ci", Value: 1}},
			Options: options.Index().SetName("idx_orgs_cityci"),
		},
		{
			Keys:    bson.D{{Key: "state_ci", Value: 1}},
			Options: options.Index().SetName("idx_orgs_stateci"),
		},
	})
}

// --- groups ---
func ensureGroups(ctx context.Context, db *mongo.Database) error {
	c := db.Collection("groups")
	return ensureIndexSet(ctx, c, []mongo.IndexModel{
		// 1) Uniqueness: no duplicate group names inside the same org (case/diacritics‑folded via name_ci)
		//    IMPORTANT: ensure you've de‑duplicated existing data before this runs, or this will fail.
		{
			Keys:    bson.D{{Key: "organization_id", Value: 1}, {Key: "name_ci", Value: 1}},
			Options: options.Index().SetUnique(true).SetName("uniq_group_org_nameci"),
		},

		// 2) Per‑org lookups and counts (used everywhere)
		{
			Keys:    bson.D{{Key: "organization_id", Value: 1}},
			Options: options.Index().SetName("idx_groups_org"),
		},

		// 3) List pages: filter by status + prefix on name_ci + stable tiebreak
		{
			Keys: bson.D{
				{Key: "organization_id", Value: 1},
				{Key: "status", Value: 1},
				{Key: "name_ci", Value: 1},
				{Key: "_id", Value: 1},
			},
			Options: options.Index().SetName("idx_groups_org_status_nameci__id"),
		},

		// Removed: leaders/members array indexes (now handled via group_memberships)
		// - idx_groups_leaders_nameci__id
		// - idx_groups_leaders_metastatus_nameci__id
		// - idx_groups_members
		// - idx_groups_members_metastatus
	})
}

func ensureGroupMemberships(ctx context.Context, db *mongo.Database) error {
	c := db.Collection("group_memberships")
	return ensureIndexSet(ctx, c, []mongo.IndexModel{
		// Uniqueness: exactly one membership per (user, group) — role is scalar; update the doc to change role
		{
			Keys:    bson.D{{Key: "user_id", Value: 1}, {Key: "group_id", Value: 1}},
			Options: options.Index().SetUnique(true).SetName("uniq_gm_user_group"),
		},

		// Fast: list group members (+role segmentation, stable tiebreak by user_id)
		{
			Keys:    bson.D{{Key: "group_id", Value: 1}, {Key: "role", Value: 1}, {Key: "user_id", Value: 1}},
			Options: options.Index().SetName("idx_gm_group_role_user"),
		},

		// Fast: list a user's groups (+role segmentation, stable tiebreak by group_id)
		{
			Keys:    bson.D{{Key: "user_id", Value: 1}, {Key: "role", Value: 1}, {Key: "group_id", Value: 1}},
			Options: options.Index().SetName("idx_gm_user_role_group"),
		},

		// Org-scoped dashboards and counts
		{
			Keys:    bson.D{{Key: "org_id", Value: 1}, {Key: "role", Value: 1}, {Key: "group_id", Value: 1}},
			Options: options.Index().SetName("idx_gm_org_role_group"),
		},
	})
}

func ensureResources(ctx context.Context, db *mongo.Database) error {
	c := db.Collection("resources")
	return ensureIndexSet(ctx, c, []mongo.IndexModel{
		// Enforce unique resource titles (case-insensitive via title_ci)
		{
			Keys: bson.D{
				{Key: "title_ci", Value: 1},
			},
			Options: options.Index().
				SetUnique(true).
				SetName("uniq_resources_titleci"),
		},
		// Status + title_ci + _id listing index
		{
			Keys: bson.D{
				{Key: "status", Value: 1},
				{Key: "title_ci", Value: 1},
				{Key: "_id", Value: 1},
			},
			Options: options.Index().
				SetName("idx_resources_status_titleci__id"),
		},
		// Subject_ci index
		{
			Keys: bson.D{
				{Key: "subject_ci", Value: 1},
			},
			Options: options.Index().
				SetName("idx_resources_subjectci"),
		},
		// Type index
		{
			Keys: bson.D{
				{Key: "type", Value: 1},
			},
			Options: options.Index().
				SetName("idx_resources_type"),
		},
	})
}

func ensureGroupResourceAssignments(ctx context.Context, db *mongo.Database) error {
	c := db.Collection("group_resource_assignments")
	return ensureIndexSet(ctx, c, []mongo.IndexModel{
		// (group_id, resource_id) index (non-unique, allows multiple assignments per resource per group)
		{
			Keys: bson.D{
				{Key: "group_id", Value: 1},
				{Key: "resource_id", Value: 1},
			},
			Options: options.Index().
				SetName("idx_assign_group_resource"),
		},
		// group_id index
		{
			Keys: bson.D{
				{Key: "group_id", Value: 1},
			},
			Options: options.Index().
				SetName("idx_assign_group"),
		},
		// resource_id index
		{
			Keys: bson.D{
				{Key: "resource_id", Value: 1},
			},
			Options: options.Index().
				SetName("idx_assign_resource"),
		},
		// (group_id, created_at) index
		{
			Keys: bson.D{
				{Key: "group_id", Value: 1},
				{Key: "created_at", Value: -1},
			},
			Options: options.Index().
				SetName("idx_assign_group_created"),
		},
		// (resource_id, created_at) index
		{
			Keys: bson.D{
				{Key: "resource_id", Value: 1},
				{Key: "created_at", Value: -1},
			},
			Options: options.Index().
				SetName("idx_assign_resource_created"),
		},
	})
}

// Helpful for dashboards that show recent activity / login lists.
func ensureLoginRecords(ctx context.Context, db *mongo.Database) error {
	c := db.Collection("login_records")
	return ensureIndexSet(ctx, c, []mongo.IndexModel{
		// Per-user recent logins (latest-first)
		{
			Keys:    bson.D{{Key: "user_id", Value: 1}, {Key: "created_at", Value: -1}},
			Options: options.Index().SetName("idx_logins_user_created"),
		},
		// Site-wide recent logins (latest-first)
		{
			Keys:    bson.D{{Key: "created_at", Value: -1}},
			Options: options.Index().SetName("idx_logins_created"),
		},

		// OPTIONAL: rate-limit/analytics by IP in a time window
		// Uncomment if you query by IP + created_at
		// {
		//     Keys:    bson.D{{Key: "ip", Value: 1}, {Key: "created_at", Value: -1}},
		//     Options: options.Index().SetName("idx_logins_ip_created"),
		// },

		// OPTIONAL TTL: auto-expire old login records after 180 days
		// Keep commented unless you want automatic pruning.
		// {
		//     Keys: bson.D{{Key: "created_at", Value: 1}},
		//     Options: options.Index().
		//         SetExpireAfterSeconds(180 * 24 * 60 * 60).
		//         SetName("idx_logins_created_ttl_180d"),
		// },
	})
}

func ensureMaterials(ctx context.Context, db *mongo.Database) error {
	c := db.Collection("materials")
	return ensureIndexSet(ctx, c, []mongo.IndexModel{
		// Enforce unique material titles (case-insensitive via title_ci)
		{
			Keys: bson.D{
				{Key: "title_ci", Value: 1},
			},
			Options: options.Index().
				SetUnique(true).
				SetName("uniq_materials_titleci"),
		},
		// Status + title_ci + _id listing index
		{
			Keys: bson.D{
				{Key: "status", Value: 1},
				{Key: "title_ci", Value: 1},
				{Key: "_id", Value: 1},
			},
			Options: options.Index().
				SetName("idx_materials_status_titleci__id"),
		},
		// Type index
		{
			Keys: bson.D{
				{Key: "type", Value: 1},
			},
			Options: options.Index().
				SetName("idx_materials_type"),
		},
	})
}

func ensureMaterialAssignments(ctx context.Context, db *mongo.Database) error {
	c := db.Collection("material_assignments")
	return ensureIndexSet(ctx, c, []mongo.IndexModel{
		// Organization-wide assignments lookup
		{
			Keys: bson.D{
				{Key: "organization_id", Value: 1},
			},
			Options: options.Index().
				SetName("idx_matassign_org"),
		},
		// Individual leader assignments lookup
		{
			Keys: bson.D{
				{Key: "leader_id", Value: 1},
			},
			Options: options.Index().
				SetName("idx_matassign_leader"),
		},
		// Material lookup (for cascade deletes and listing assignments per material)
		{
			Keys: bson.D{
				{Key: "material_id", Value: 1},
			},
			Options: options.Index().
				SetName("idx_matassign_material"),
		},
		// Combined lookup: all assignments for a leader (both org-wide and individual)
		// This supports the query: (organization_id = leaderOrgID) OR (leader_id = leaderID)
		{
			Keys: bson.D{
				{Key: "organization_id", Value: 1},
				{Key: "leader_id", Value: 1},
			},
			Options: options.Index().
				SetName("idx_matassign_org_leader"),
		},
	})
}

func ensurePages(ctx context.Context, db *mongo.Database) error {
	c := db.Collection("pages")
	return ensureIndexSet(ctx, c, []mongo.IndexModel{
		// Unique slug for each page (about, contact, terms-of-service, privacy-policy)
		{
			Keys: bson.D{
				{Key: "slug", Value: 1},
			},
			Options: options.Index().
				SetUnique(true).
				SetName("uniq_pages_slug"),
		},
	})
}
