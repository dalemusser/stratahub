// migrate_user_auth_fields.js
// Migration script to update user records with new auth fields structure
//
// Run with: mongosh stratahub scripts/migrate_user_auth_fields.js
//
// This script:
// 1. Copies email -> login_id (lowercase)
// 2. Creates login_id_ci (folded for case/diacritic-insensitive matching)
// 3. Sets auth_return_id to null
// 4. Sets auth_method to "trust"
// 5. Sets email to null

// Text folding function - removes diacritics and lowercases
function fold(str) {
  if (!str) return "";
  return str
    .normalize("NFD")
    .replace(/[\u0300-\u036f]/g, "")
    .toLowerCase();
}

// Get the database
const dbName = "strata_hub";
const db = db.getSiblingDB(dbName);

print(`\n=== User Auth Fields Migration ===`);
print(`Database: ${dbName}`);
print(`Collection: users\n`);

// Remove the validator to allow schema changes
print("Removing collection validator...");
db.runCommand({
  collMod: "users",
  validator: {},
  validationLevel: "off"
});
print("Validator disabled.");

// Drop the unique email index (we'll create login_id_ci index later)
print("Dropping unique email index...");
try {
  db.users.dropIndex("uniq_users_email");
  print("Index dropped.");
} catch (e) {
  print("Index not found or already dropped: " + e.message);
}
print("");

// Count users before migration
const totalUsers = db.users.countDocuments({});
print(`Total users to migrate: ${totalUsers}\n`);

if (totalUsers === 0) {
  print("No users found. Exiting.");
  quit();
}

// Show sample of current data
print("Sample of current user data (first 3):");
db.users.find({}, { email: 1, auth_method: 1 }).limit(3).forEach(doc => {
  print(`  _id: ${doc._id}, email: "${doc.email}", auth_method: "${doc.auth_method}"`);
});
print("");

// Perform the migration
let migrated = 0;
let errors = 0;

db.users.find({}).forEach(doc => {
  try {
    const email = doc.email || "";
    const loginId = email.toLowerCase();
    const loginIdCi = fold(email);

    db.users.updateOne(
      { _id: doc._id },
      {
        $set: {
          login_id: loginId,
          login_id_ci: loginIdCi,
          auth_return_id: null,
          auth_method: "trust",
          email: null
        }
      }
    );
    migrated++;
  } catch (e) {
    print(`Error migrating user ${doc._id}: ${e.message}`);
    errors++;
  }
});

print(`\n=== Migration Complete ===`);
print(`Migrated: ${migrated}`);
print(`Errors: ${errors}`);

// Show sample of migrated data
print("\nSample of migrated user data (first 3):");
db.users.find({}, { login_id: 1, login_id_ci: 1, auth_return_id: 1, auth_method: 1, email: 1 }).limit(3).forEach(doc => {
  print(`  _id: ${doc._id}`);
  print(`    login_id: "${doc.login_id}"`);
  print(`    login_id_ci: "${doc.login_id_ci}"`);
  print(`    auth_return_id: ${doc.auth_return_id}`);
  print(`    auth_method: "${doc.auth_method}"`);
  print(`    email: ${doc.email}`);
});

// Verify all records have the new fields
const withNewFields = db.users.countDocuments({
  login_id: { $exists: true },
  login_id_ci: { $exists: true },
  auth_return_id: { $exists: true },
  auth_method: "trust",
  email: null
});
print(`\nVerification: ${withNewFields} of ${totalUsers} users have new field structure.`);
