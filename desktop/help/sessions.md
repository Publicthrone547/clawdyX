# Sessions and tiers

## Sessions

A session is one Claude Code conversation in one workspace (a project folder you allowed). The bot resumes it for every message, so Claude remembers the conversation. **Sessions** in this app lists them; you can create, switch, rename, archive, restore and forget them.

| Command | clawdyX session list | Claude Code history on disk |
| --- | --- | --- |
| `/new` | adds a session and makes it active | untouched |
| `/clear` | a fresh session with the same workspace and settings | untouched |
| `/newf` | adds a session and forgets the previous one (restorable for 30 days) | untouched |
| `/forget S2 --delete` | forgets it | deletes exactly that session's transcript, after a confirmation that names it |

clawdyX deletes nothing else of Claude Code's.

## Permission tiers

Every session has a tier. It decides what Claude may do without asking.

| Tier | Claude may | Claude may not |
| --- | --- | --- |
| `observe` (default) | read files in the workspace, run read-only commands | edit files, run other commands, use the web |
| `assisted` | the above, plus edit files in the workspace and run the workspace's allowed commands | {{.W.DeniedSummary}}, read outside the workspace, touch secret paths |
| `automation` (opt-in) | run commands anywhere on {{.W.ThisComputer}} ({{.W.AutomationExamples}}) when Claude Code's safety review approves them | the same deny list, plus secret paths |

Automation must first be allowed in **Settings → Features → Automation tier available**, and each session needs a confirmation button (`/mode automation`). `/mode observe` switches back. Claude Code's own bypass mode stays off in every tier.

Secret places are denied in every tier: SSH and cloud keys, browser profiles, {{.W.TokenStoreShort}} data, and Claude Code's own folder. Claude reports such reads as denied and carries on.

## Models and effort

`/model` and `/effort` change them for the active session (`--default` for new sessions). Every model change is checked with a short test before it is saved.
