// The wording guard's matcher, the same rules as internal/platform/forbidden.go:
// case-sensitive whole words (a boundary only where the word starts or ends
// with a letter or digit), allowed phrases removed first.
import data from "./forbidden.json";

export type OS = "darwin" | "windows" | "linux";
export const oses: OS[] = ["darwin", "windows", "linux"];

const escape = (s: string) => s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
const isWord = (c: string) => /[A-Za-z0-9_]/.test(c);

function wordRe(w: string): RegExp {
  let e = escape(w);
  if (isWord(w[0])) e = "\\b" + e;
  if (isWord(w[w.length - 1])) e += "\\b";
  return new RegExp(e);
}

const lists = data.forbidden as Record<OS, string[]>;
const allowed = data.allowed as Record<string, Record<string, string>>;

/** forbiddenWords returns the words of os's list found in text. */
export function forbiddenWords(os: OS, text: string): string[] {
  const ok = [...Object.keys(allowed.all ?? {}), ...Object.keys(allowed[os] ?? {})].sort((a, b) => b.length - a.length);
  for (const a of ok) text = text.split(a).join(" ");
  return lists[os].filter((w) => wordRe(w).test(text));
}
