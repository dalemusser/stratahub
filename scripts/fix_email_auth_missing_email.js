// fix_email_auth_missing_email.js
//
// Fixes users with auth_method: "email" but email: null (or missing).
// For email-auth users, the login_id IS their email address, so we
// copy login_id into the email field.
//
// Usage:
//   mongosh <connection-string> --eval 'var dryRun=true' fix_email_auth_missing_email.js
//   mongosh <connection-string> --eval 'var dryRun=false' fix_email_auth_missing_email.js
//
// Default is dry run (preview only).

if (typeof dryRun === "undefined") {
  dryRun = true;
}

print("=== Fix email-auth users missing email field ===");
print("Mode: " + (dryRun ? "DRY RUN (no changes)" : "LIVE (will update records)"));
print("");

// Find affected users
const filter = {
  auth_method: "email",
  $or: [{ email: null }, { email: { $exists: false } }],
};

const affected = db.users
  .find(filter, { login_id: 1, email: 1, auth_method: 1, role: 1, full_name: 1 })
  .toArray();

print("Found " + affected.length + " affected user(s):");
print("");

affected.forEach(function (u) {
  print(
    "  _id: " + u._id +
    "  login_id: " + u.login_id +
    "  role: " + u.role +
    "  full_name: " + (u.full_name || "(none)")
  );
});

if (affected.length === 0) {
  print("Nothing to fix.");
  quit();
}

print("");

if (dryRun) {
  print("Dry run complete. Re-run with dryRun=false to apply changes.");
} else {
  const result = db.users.updateMany(
    filter,
    [{ $set: { email: "$login_id", updated_at: new Date() } }]
  );
  print("Updated " + result.modifiedCount + " user(s).");

  // Verify
  print("");
  print("Verification — these users should now have email set:");
  affected.forEach(function (u) {
    const updated = db.users.findOne({ _id: u._id }, { login_id: 1, email: 1 });
    print("  _id: " + updated._id + "  email: " + updated.email + "  login_id: " + updated.login_id);
  });
}
