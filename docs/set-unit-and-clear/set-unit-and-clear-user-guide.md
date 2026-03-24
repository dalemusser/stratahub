# Set Unit & Clear Downloads — User Guide

This guide explains how to use the **Set to Unit** and **Clear All Downloads** features in Mission HydroSci.

## What do these features do?

**Set to Unit** changes a student's current unit in Mission HydroSci. When you set a student to a particular unit, all units before it are automatically marked as completed. For example, setting to Unit 3 marks Units 1 and 2 as completed.

Setting to Unit 1 resets all progress — nothing is marked as completed.

**No data is deleted.** Setting a unit only changes the pointer to which unit the student is playing. All save data, logging data, grades, and scores are preserved.

**Clear All Downloads** removes all cached unit files from the device. This frees up storage space but does not change the student's progress. The student will need to re-download units before playing them offline.

These are separate actions. You can change a student's unit without affecting their downloads, and you can clear downloads without changing their progress.

---

## For Leaders and Admins: MHS Dashboard

### Setting a student's unit

1. Open the **MHS Dashboard** and select a group.
2. Go to the **Progress** tab.
3. Find the student's name in the left column and click the **settings icon** next to their name.
4. A menu will appear with **Set to Unit** options — Unit 1 through Unit 5.
5. Click the desired unit.
6. A confirmation dialog will appear: *"Set to Unit X for [Student Name]? All prior units will be marked completed."*
7. Click **OK** to confirm. The grid will refresh to show the updated progress.

### Understanding the progress indicators

The progress grid shows two colored bars on each student's row to indicate their current unit:

- **Green bar** (top of cell, labeled "MHS Unit" in the legend) — Shows the unit the student is currently assigned to in Mission HydroSci. This is the unit that changes when you use Set to Unit.

- **Black/white bar** (bottom of cell, labeled "Playing Unit" in the legend) — Shows the unit the student last played in the game. It updates automatically when the student starts a unit.

These two indicators can show different units. For example, if you set a student to Unit 4 but they haven't launched the game yet, the green bar will show Unit 4 while the other bar still shows whichever unit they last played. Once the student opens Unit 4 in the game, both bars will update to match.

Hover over either bar to see a tooltip explaining what it represents.

### Tips

- The grid auto-refreshes every 30 seconds. If you have the settings menu open, the refresh will wait until you close it.
- After setting a unit, the grid refreshes immediately to show the change.
- You can only set units for students who are members of the currently selected group.

---

## For All Users: Mission HydroSci Units Page

### Setting your unit

1. Open **Mission HydroSci** and go to the units page.
2. Expand the **Manage downloads** section.
3. At the bottom, you'll see a bordered box with **Set to** followed by a dropdown and a **Go** button.
4. Select the unit you want from the dropdown (units are listed with their full titles, e.g., "Unit 2: Water Quality").
5. Click **Go**.
6. Confirm the action (see Authorization below).
7. The page will reload showing your updated progress.

### Clearing all downloads

1. Open **Mission HydroSci** and go to the units page.
2. Expand the **Manage downloads** section.
3. At the bottom of the bordered box, click the red **Clear All Downloads** button.
4. Confirm the action (see Authorization below).
5. All cached unit files will be removed from the device. The page will reload.

This does **not** change your progress. Your current unit and completed units remain the same. You will need to re-download units before playing them offline.

You can also clear individual unit downloads using the **Clear** button on each unit card, rather than clearing everything at once.

---

## Authorization

Setting a unit and clearing downloads are actions that require confirmation. The confirmation process depends on your role and how your StrataHub workspace is configured.

### Leaders, admins, and coordinators

You will see a simple confirmation dialog. Click **OK** to proceed.

### Members (students)

Your workspace administrator chooses what level of authorization is required for members. There are three options:

#### Staff Auth (default)

A leader, coordinator, admin, or superadmin must authenticate on your behalf:

1. An authorization dialog will appear asking for the staff member's login ID.
2. The staff member enters their login ID.
3. Depending on their account setup, they may need to:
   - Enter their password, or
   - Enter a verification code sent to their email
4. Once authenticated, the action proceeds.

This is the default and most secure option. It ensures a staff member is present and approves the action.

#### Keyword

A keyword dialog will appear. Enter the keyword provided by your leader or admin and click **Confirm**. If the keyword is incorrect, an error message will appear. You can click **Cancel** or press **Escape** to dismiss the dialog without making changes.

#### Trust

No additional authorization is required. You will see a simple confirmation dialog, the same as leaders and admins.

---

## Frequently Asked Questions

**Can I undo a Set to Unit action?**
There is no undo button, but you can set to a different unit at any time. For example, if you accidentally set to Unit 1 (which resets all progress), you can set back to the unit the student was on.

**Will setting a unit affect the student's grades or scores?**
No. Setting a unit changes which unit the student is assigned to and which units are marked as completed. It does not erase any grades, scores, save data, or logging data that have already been recorded. All data is kept.

**Why do the two progress bars on the dashboard show different units?**
The green bar shows the unit assigned in Mission HydroSci (updated by Set to Unit), while the red bar shows the unit last played in the game (updated automatically when the student starts a unit). They will match once the student opens the assigned unit in the game.

**Does Clear All Downloads affect my progress?**
No. Clearing downloads only removes the cached files used for offline play. Your current unit and completed units are unchanged.

**Can members set their own unit?**
Yes, from the Mission HydroSci units page. The authorization required depends on the workspace setting — staff auth (default), keyword, or trust.

**Who configures the authorization mode?**
A workspace administrator sets the member authorization mode in the StrataHub workspace settings. The default is staff auth.
