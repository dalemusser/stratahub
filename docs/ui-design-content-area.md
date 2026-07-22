# Content-Area Layout — StrataHub Design Parameter

**The rule:** every page's main surface (the white/dark-gray elevated panel) extends to the
edges of the content area, with a margin of page background visible on the left, right, and
bottom. The surface **fills the available height** — it reaches down to the bottom margin
even when its content is short (see Organizations with only 9 rows).

Reference implementations: `internal/app/features/organizations/templates/organizations_list.gohtml`,
`resources`. Fuller documentation lives in the template repo:
`stratasave/docs/ui_design.md` and `stratasave/docs/ui-design-patterns.md`.

## The pattern

```html
{{ define "content" }}
<div class="flex flex-col h-full">                        <!-- page fills #content -->

  <!-- Header row: sits on the page background, outside any surface -->
  <div class="mb-4 flex items-center justify-between">
    <h1 class="text-2xl font-bold text-gray-900 dark:text-gray-100">🏢 Title</h1>
    <a class="px-3 py-1 text-sm bg-indigo-600 text-white rounded hover:bg-indigo-700">Action</a>
  </div>

  <section class="flex-1 min-w-0 flex flex-col">

    <!-- Optional small surface(s): search bar, toolbars — natural height -->
    <form class="bg-white dark:bg-gray-800 rounded shadow p-3 mb-1 ...">…</form>

    <!-- Main surface: flex-1 makes it FILL the remaining height;
         mb-2 (or mb-4) is the bottom margin -->
    <div class="p-4 bg-white dark:bg-gray-800 rounded shadow flex-1 mb-2 overflow-auto">
      …content, typically a full-width table…
    </div>

  </section>
</div>
{{ end }}
```

## How it works

- `#content` in `layout.gohtml` has `px-4 py-2 overflow-y-auto flex-1` — its horizontal
  padding **is** the left/right margin; never add your own page-level gutters.
- The page root `flex flex-col h-full` lets children stretch vertically.
- Exactly one surface per page carries `flex-1` — usually the biggest/list-like one — so
  the dark area reaches the bottom. Its `mb-2`/`mb-4` is the bottom margin.
- Surfaces are full width (`min-w-0` on the section prevents flex overflow). If a page's
  *content* should be narrower for readability (e.g., a launcher button), constrain a
  wrapper **inside** the surface (`max-w-lg mx-auto`), never the surface itself.
- Surface style: `bg-white dark:bg-gray-800 rounded shadow` + padding. Inner list items
  on a surface use the muted inset style `bg-gray-50 dark:bg-gray-700/50 rounded`.
- Tables follow the Organizations idiom: `min-w-full text-sm text-left`, header
  `bg-gray-100 dark:bg-gray-700 … uppercase text-xs`, rows with
  `border-b … hover:bg-gray-50 dark:hover:bg-gray-900/50`, right-aligned Actions column.

## Gotcha: bottom spacing on pages that scroll

Chrome excludes the scroll container's own `padding-bottom` (and drops trailing spacing
that lives on a fixed-height section) from the scrollable overflow. So on a page tall
enough to scroll, `#content`'s `py-2` bottom padding never scrolls into view and the last
surface can end up nearly touching the footer, even though short pages look right.

The fix used on the MHS pages: an explicit trailing spacer as the last flex item in the
section — a real box always counts in the scrollable area:

```html
    <div class="h-4 shrink-0" aria-hidden="true"></div>
  </section>
```

Size the spacer to the page's dominant state — the two states always differ by the
container's 8px padding, so pick the one users actually see:

- **Pages that scroll in practice** (content taller than the viewport, e.g. MHS manage):
  `h-4` → 16px scrolled (matches the sides), 24px in the rare short case.
- **Pages that fit the viewport** (a `flex-1` surface absorbs the height, e.g. the MHS
  launcher): `h-2` → 8px spacer + the container's visible 8px padding = 16px.

## Anti-pattern (what Mission HydroSci did before 2026-07)

Narrow `max-w-lg mx-auto` cards floating on the page background, nothing filling the
height — the page background dominated and the layout didn't match the rest of the app.
Fixed in `missionhydrosci_units.gohtml` / `missionhydrosci_manage.gohtml`, which now
serve as additional examples of the pattern.
