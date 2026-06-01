# Draft email to Abt — Strata de-identification for surveys

_Revised draft for the director to send to Abt. Key correction from the original draft: **Strata does not store survey response data** — Abt continues to store that. Strata supplies the de-identified ID on each survey link and holds the authorized identity crosswalk (the Members Report). A short, to-the-point summary has been added near the top._

---

Hi...

We're working to make Strata as strong as possible for delivering games and other electronic resources to students - first in the context of impact studies, and later into full implementation - and to do it in compliance with FERPA / COPPA / IRB requirements.

The short version:  your surveys will receive the info about each student as a de-identified hex object ID, along with de-identified IDs for their class, school, and workspace. You collect and store your survey data keyed to those IDs. The IDs are provided as url params just like the previous information you received. When it is necessary and authorized to connect an ID back to a real student, teacher, or class, Strata's Members Report provides that crosswalk as a downloadable csv.

A bit more detail:

Dale has built a de-identification system that covers both game play and assessments/surveys. Strata assigns a stable object identifier (a 24-character hex ID) to each student, and that ID - not a name or login - is what travels on every resource link a student opens, including your survey links. Strata also sends de-identified IDs for the student's group (class), organization (school), and workspace. Strata keeps the table that maps those IDs to identities internally, and that table is never exposed outside Strata except as a downloadable CSV from Members Report. This lets Abt collect survey data associated with de-identified IDs matched to school, teacher, and class, with no identifiable information in the data sent to you in the url params.

What Strata contributes is (1) the de-identified ID on each survey link and (2) the authorized crosswalk back to identity. Because the in-game data Strata captures and your survey data both use the same de-identified student ID, our MHS team can correlate player gains and attitudes (from your surveys) with player behavior (from the game) without any identifiable information moving between systems. And because Strata holds the crosswalk, we can also produce teacher-facing reports that include student names and outcomes inside Strata, where that is appropriate.

If your team would like to talk through how this works, we could move our MHS–Abt meeting up to June 9 and have Dale join.

I'd like to get this general concept into an IRB amendment to submit next week, then update our data use agreement as needed, and likely file a second IRB amendment near the middle of the summer to match any updates to the DUA.

Let us know what questions or concerns you have.

Dale's info:

URL Identity Parameters - Guide for Admins & Coordinators
The guide for admins and coordinators about creating and editing Resources to include URL parameters for data collection.
https://github.com/dalemusser/stratahub/blob/main/docs/resource-identification/admin-coordinator-guide.md

URL Identity Parameters - Guide for Data Recipients
The guide for analysts/researchers who receive StrataHub identity information via query parameters on a resource's launch URL (surveys, etc.).
https://github.com/dalemusser/stratahub/blob/main/docs/resource-identification/data-consumer-guide.md

Resource URL Identification - Parameter Vocabulary (Permanent Contract)
Documentation of the URL parameters, referenced by the Guide for Data Recipients above.
https://github.com/dalemusser/stratahub/blob/main/docs/resource-identification/vocabulary.md

Members Report - Resolving Identity from Hex IDs
The de-identified URL schemes (and any data a consumer collects keyed on them) identify a student only by opaque 24-char hex IDs: user_id, group_id, org_id, ws_id. The Members Report in StrataHub is the authorized crosswalk that maps those same hex IDs back to names, organizations, groups, and logins. It is an updated version of the previous membership report, now including the workspace and the object IDs for all of the involved entities.
https://github.com/dalemusser/stratahub/blob/main/docs/resource-identification/members-report.md

Let me know if you have any questions or need anything else.
