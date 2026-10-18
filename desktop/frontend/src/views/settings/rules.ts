// Permission rules as Claude Code reads them: Tool(specifier), for example
// Bash(npm test *). The worker validates again when saving.
const ruleRe = /^[A-Za-z][A-Za-z0-9_]*\([^\r\n]*\)$/;
const toolRe = /^[A-Za-z][A-Za-z0-9_]*$/;

/** checkRule returns "" for a valid rule, else the reason. */
export function checkRule(v: string, bareTools = false): string {
  if (ruleRe.test(v) || (bareTools && toolRe.test(v))) return "";
  return bareTools ? "Write a rule like Bash(npm publish *) or a tool name like NotebookEdit." : "Write a rule like Bash(npm test *).";
}

export interface Preset {
  name: string;
  allowed: string[];
}

// Presets for a workspace's allowed commands (assisted tier). Claude
// Code's shell is bash on every OS (Git Bash on Windows), so they are the
// same everywhere.
export const presets: Preset[] = [
  { name: "Go project", allowed: ["Bash(go test *)", "Bash(go build *)", "Bash(go vet *)", "Bash(gofmt -l *)"] },
  { name: "Node project", allowed: ["Bash(npm test *)", "Bash(npm run *)", "Bash(npx tsc *)", "Bash(node *)"] },
  { name: "Python project", allowed: ["Bash(pytest *)", "Bash(python -m pytest *)", "Bash(ruff *)", "Bash(mypy *)"] },
  { name: "Git (read-only)", allowed: ["Bash(git status *)", "Bash(git diff *)", "Bash(git log *)", "Bash(git show *)"] },
];

// The rules every workspace must deny unless it opts out (config.MinimumDenied).
export const minimumDenied = ["Bash(rm *)", "Bash(sudo *)", "Bash(git push *)"];
