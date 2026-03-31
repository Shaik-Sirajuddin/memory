# Official Changeset Format (Minimal)

This repo uses the [Changesets](https://github.com/changesets/changesets) markdown format.

## File location

Create one file per release intent under:

```
.changeset/<unique-id>.md
```

## File shape

```md
---
"mem": patch
---

Short summary of the change.
```

## Notes

- Front matter is YAML.
- Key is the npm package name (`"mem"` in this project).
- Allowed bump values are `patch`, `minor`, `major`.
- Body supports normal markdown and becomes changelog content during release.
- Recommended command to create one interactively:

```bash
npx @changesets/cli
```

