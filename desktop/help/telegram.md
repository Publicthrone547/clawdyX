# Telegram

## The bot and its token

Create the bot with [@BotFather](open:botfather): send `/newbot`, pick a name, and copy the token it gives you. The token is the bot's password. Never paste it into a chat, an issue or a file in a repository. clawdyX keeps it in {{.W.TokenStore}} and shows only its last four characters.

To replace it (after `/revoke` in @BotFather, for example), use **Telegram → Bot token → Replace token…** in this app. The new token is checked with Telegram before it is stored, and the worker restarts with it.

## Who may use the bot

Only the Telegram users on your allowlist are served, by numeric ID, in their own private chat. Everyone else is ignored silently: no reply, nothing stored.

| Role | Can | Cannot |
| --- | --- | --- |
| `owner` (exactly one) | everything | — |
| `user` | chat with Claude, list, create, switch and rename sessions, set the model and effort of the active session, stop their own jobs | change tiers or defaults, use screens, the camera or `/open`, forget sessions, `/doctor`, `/processes`, `/logs`, pause or shut down the bot |

To add someone, open **Telegram → Allowed Telegram users**, click **Detect from a message…**, and let them send any message to the bot. Nothing is recorded unless detection is armed, and nobody gets a reply.

## Commands

| Area | Commands |
| --- | --- |
| Chat | any plain message; long jobs stream a preview and can be cancelled with the button |
| Sessions | `/status` `/sessions` `/new` `/newf` `/clear` `/switch S2` `/rename S2 name` `/forget S2` |
| Claude | `/model` `/effort` `/mode observe` `/mode assisted` `/mode automation` |
| Jobs | `/stop` `/cancel j-ID` `/jobs` `/logs j-ID` |
| {{.W.OSName}} | `/processes` `/usage` `/doctor` `/open example.com` (opens a website in the default browser of {{.W.ThisComputer}}) |
| Screens and camera | `/screens` `/screen 2` `/look 2 question` `/screen off` `/cameras` `/camera` (see [Screens and camera](help:screens)) |
| Bot | `/start` `/pause` `/confirm` `/deny` |

## Connection

**Telegram** in this app shows whether the bot is polling and how many messages were received, sent and dropped. "Duplicate poller detected" means another program polls with the same token: a second copy of clawdyX, or a leaked token (replace it).
