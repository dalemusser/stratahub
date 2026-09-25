// One-time backfill of the end-of-game mark on mhs_user_progress (2026-09-25).
//
// The end-of-game ceremony hangs off game_ended_at (set by the game's EndGame
// or by a staff "End of game" jump; cleared by any set-to-unit). Records that
// were already "complete" when the mark was introduced were completed by the
// game's own end path, so they get the mark dated from their last update.
// Records at unit5 or earlier are left alone by design: a student whose
// progress never reached "complete" ends the game by playing to the end
// again or by a staff jump (dashboard menu or manage page).
//
// Run against the stratahub database with mongosh (a dry run lists only):
//   mongosh --quiet "<stratahub URI>" scripts/backfill_mhs_game_ended.js
//   BACKFILL_APPLY=1 mongosh --quiet "<stratahub URI>" scripts/backfill_mhs_game_ended.js
const apply = (typeof process !== 'undefined' && process.env && process.env.BACKFILL_APPLY === '1');
const coll = db.getSiblingDB('stratahub').mhs_user_progress;
const filter = { current_unit: 'complete', game_ended_at: { $exists: false } };

const total = coll.countDocuments({});
const marked = coll.countDocuments({ game_ended_at: { $exists: true } });
const candidates = coll.find(filter, { workspace_id: 1, user_id: 1, updated_at: 1, completed_units: 1 }).toArray();
print(`mhs_user_progress: ${total} records, ${marked} already marked, ${candidates.length} complete without the mark`);
candidates.forEach(c => print(`  ${c._id}  ws=${c.workspace_id}  user=${c.user_id}  updated=${c.updated_at ? c.updated_at.toISOString() : '-'}  completed=${(c.completed_units || []).length}`));

if (!apply) {
  print('dry run — set BACKFILL_APPLY=1 to write');
} else {
  let n = 0;
  candidates.forEach(c => {
    const at = c.updated_at || new Date();
    const r = coll.updateOne({ _id: c._id, game_ended_at: { $exists: false } },
      { $set: { game_ended_at: at, game_ended_by: 'backfill' } });
    n += r.modifiedCount;
  });
  print(`marked ${n} records (game_ended_by: "backfill")`);
  print(`now marked: ${coll.countDocuments({ game_ended_at: { $exists: true } })}`);
}
