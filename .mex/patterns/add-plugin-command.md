---
name: add-plugin-command
description: Adding an Obsidian plugin command, modal, or settings surface (publish flow UI work).
triggers:
  - "command palette"
  - "modal"
  - "plugin settings"
  - "obsidian command"
  - "add command"
edges:
  - target: context/architecture.md
    condition: to see the user flow the command participates in (PRD §7)
  - target: patterns/add-api-endpoint.md
    condition: when the command needs a new server API call
grounds_to: []
last_updated: 2026-09-10
---

# Add a plugin command / modal

## Context
Plugin UI per `PRD.md` §7–8: commands (command palette + note context menu), modals (publish modal with route + password fields), and settings tabs (server config, published-pages list, unpublish, theme). Published-state UI is always sourced live from the server — the local note↔route map is a convenience cache, never truth.

## Steps
1. Register the command via the plugin's `addCommand` (and `addRibbonIcon`/menu registration if the flow calls for it).
2. Build the modal/settings tab with Obsidian's API (`Modal`, `Setting`). Prefill from server state where the flow shows existing state (e.g. re-publish modal shows current route/password state).
3. Call the plugin API client; never fetch with raw `request()` bypassing the shared auth header.
4. Give explicit success/error feedback (Notice) — distinguish invalid token, route taken, server unreachable (PRD P9).
5. Include copy-to-clipboard where a URL is produced (PRD P10).

## Gotchas
- [VERIFY AFTER FIRST IMPLEMENTATION] File layout for commands vs modals vs settings.
- Never render the settings page list from the local map alone — fetch live from `GET /api/pages`.
- Don't persist note content locally; only note↔route metadata.
- Password field in the publish modal: don't echo a stored password back on re-publish (server only has the hash) — offer "set/change/remove" semantics instead.

## Verify
- [ ] Command appears in palette (and context menu where specified)
- [ ] Errors surface as Notices with distinct messages per failure mode
- [ ] UI state for published pages comes from the server
- [ ] No token or password written to console/plugin data beyond what settings must store

## Debug
- Command missing from palette: check `addCommand` registration and plugin enable state.
- Modal data stale: it was sourced from the local map — switch to a live server fetch.

## Update Scaffold
- [ ] Update `.mex/ROUTER.md` "Current Project State" if what's working/not built has changed
- [ ] Update any `.mex/context/` files that are now out of date
- [ ] If this is a new task type without a pattern, create one in `.mex/patterns/` and add to `INDEX.md`
