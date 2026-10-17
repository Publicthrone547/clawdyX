// Per-viewer UI conveniences only (theme, last screen). Nothing here is
// security relevant; storage may be unavailable and that is fine.
export type Theme = "system" | "light" | "dark";

function get(key: string, def: string): string {
  try {
    return localStorage.getItem("clawdyx." + key) ?? def;
  } catch {
    return def;
  }
}
function set(key: string, v: string) {
  try {
    localStorage.setItem("clawdyx." + key, v);
  } catch {
    /* ignore */
  }
}

export function theme(): Theme {
  const t = get("theme", "system");
  return t === "light" || t === "dark" ? t : "system";
}
export function setTheme(t: Theme) {
  set("theme", t);
  applyTheme();
}
export function applyTheme() {
  const t = theme();
  if (t === "system") document.documentElement.removeAttribute("data-theme");
  else document.documentElement.setAttribute("data-theme", t);
}
export const pref = { get, set };
