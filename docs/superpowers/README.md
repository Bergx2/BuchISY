# Historical Implementation Notes

This directory contains historical feature plans and design notes used while building the current Go/Fyne app.

Use these files only as archaeology:

- They may contain unchecked task lists even when the feature is already implemented.
- They may describe an older design that changed during implementation.
- They may mention branch names, subagents, or implementation process that is irrelevant to a rebuild.
- They are not the source of truth for current behavior.

For a rebuild, use:

1. `../FUNCTIONAL_SPEC.md` for normative behavior.
2. `../REBUILD_GUIDE.md` for reading order and compatibility rules.
3. `../ACCEPTANCE_MATRIX.md` for parity checks.
4. `../UI_INVENTORY.md` for screens, dialogs, menus, and visible states.

If a behavior is missing from the functional spec but appears here, verify it against current source code before treating it as real.
