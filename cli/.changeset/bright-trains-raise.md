---
"mem": patch
---

Fix `mem agent init` to resolve the active `.memory` folder from the current directory or any parent directory.

Also adds a shared resolver used across init flows and updates tests/help text for nested-path behavior.

