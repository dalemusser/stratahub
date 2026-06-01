# Strata Hub documentation — demo persona roster

A single coherent, clearly-fictional scenario for the `project.adroit.games` doc workspace, so screenshots show a believable deployment without any real data.

**Conventions**
- All demo people use the reserved **`@example.edu`** domain, which marks them unmistakably as sample data.
- **Passwords are not in this file.** They live only in `.playwright/secrets.env` (gitignored). This roster is safe to commit; credentials are not.
- The admin account already exists (**Dale Musser — admin**); the roster below is everything the build creates on top of it.

---

## Organization

**Riverbend Middle School** — the primary org for the whole walkthrough.

*(Optional second org, only if you want the Organizations list to show more than one row: **Cedar Hollow Academy**. It can stay empty.)*

## Groups (within Riverbend Middle School)

- **Grade 7 Science — Section A** — led by Marcus Webb
- **Grade 7 Science — Section B** — led by Diane Okafor

## Leaders

| Name | Email | Group |
|---|---|---|
| Marcus Webb | marcus.webb@example.edu | Section A |
| Diane Okafor | diane.okafor@example.edu | Section B |

## Members

**Section A**

| Name | Email |
|---|---|
| Aisha Rahman | aisha.rahman@example.edu |
| Liam Chen | liam.chen@example.edu |
| Sofia Delgado | sofia.delgado@example.edu |
| Devon Brooks | devon.brooks@example.edu |

**Section B**

| Name | Email |
|---|---|
| Noah Whitfield | noah.whitfield@example.edu |
| Priya Nair | priya.nair@example.edu |
| Jordan Ellis | jordan.ellis@example.edu |
| Hana Kim | hana.kim@example.edu |

## Resources (assignable content)

- Introduction to Watersheds
- The Water Cycle
- Water Quality & Testing
- Groundwater & Aquifers

Suggested assignment, so the screens aren't identical: assign all four to **Section A**, and the first two to **Section B**. (Adjust to whatever Strata Hub's assignment flow makes natural.)

## Materials (attachments)

- Watershed Field Guide (PDF)
- Water Cycle Diagram (image)

---

## Accounts to log into for role-view captures

Only two demo accounts need to be logged into (for the leader and member perspectives). Set their passwords when creating them and record them in `.playwright/secrets.env`:

- **Leader view:** Marcus Webb — `marcus.webb@example.edu`
- **Member view:** Aisha Rahman — `aisha.rahman@example.edu`

The other accounts are roster entries only (created by the admin, never logged into), so any throwaway password is fine; they don't need to be memorable.

**`.playwright/secrets.env` additions** (replacing the earlier `TEACHER_`/`STUDENT_` placeholders — Strata Hub's terms are Leader/Member):

```
LEADER_EMAIL=marcus.webb@example.edu
LEADER_PASSWORD=
MEMBER_EMAIL=aisha.rahman@example.edu
MEMBER_PASSWORD=
```

Remember to set each created account's **Theme → System** right after creation, or its light/dark capture won't flip.