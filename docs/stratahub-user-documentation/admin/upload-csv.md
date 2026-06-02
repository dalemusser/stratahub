# Upload CSV

**Upload CSV** imports several people at once from a spreadsheet file, instead of
adding them one at a time. It's the fastest way to populate a group at the start of
a term.

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="images/upload-csv-dark.png">
  <img alt="Upload CSV screen with organization and group selectors and a format reference" src="images/upload-csv-light.png">
</picture>

## Steps

1. **Select Organization** — choose which organization the people belong to.
2. **Select Group** (optional) — choose a group to add them to directly. Leave it
   blank to import people without placing them in a group.
3. **Choose File** — pick your `.csv` file.
4. **Upload & Preview** — review what will be imported before it's saved.

## CSV format

A header row is optional, and the two formats below can be mixed in the same file:

- **Email sign-in** — `full_name, auth_type, email`
  _Example:_ `Jane Doe, email, jane@school.edu`
- **Password sign-in** — `full_name, auth_type, login_id, temp_password [, email]`
  _Example:_ `John Smith, password, jsmith, TempPass1, john@school.edu`

The trailing `[, email]` is optional. People imported with **password** sign-in use
the temporary password the first time and are prompted to choose their own.
