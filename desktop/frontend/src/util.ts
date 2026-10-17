export function ago(unix: number | undefined | null): string {
  if (!unix) return "never";
  const s = Math.max(0, Math.round(Date.now() / 1000 - unix));
  if (s < 5) return "just now";
  if (s < 60) return `${s} s ago`;
  if (s < 3600) return `${Math.round(s / 60)} min ago`;
  if (s < 86400) return `${Math.round(s / 3600)} h ago`;
  return `${Math.round(s / 86400)} d ago`;
}

export function dur(ms: number): string {
  const s = Math.round(ms / 1000);
  if (s < 60) return `${s} s`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m} min ${s % 60} s`;
  return `${Math.floor(m / 60)} h ${m % 60} min`;
}

export function since(unix: number): string {
  return unix ? dur(Date.now() - unix * 1000) : "";
}

export function bytes(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1 << 20) return `${(n / 1024).toFixed(0)} KB`;
  if (n < 1 << 30) return `${(n / (1 << 20)).toFixed(1)} MB`;
  return `${(n / (1 << 30)).toFixed(1)} GB`;
}

export type Tone = "ok" | "warn" | "danger" | "muted" | "accent";

export function jobTone(state: string): Tone {
  switch (state) {
    case "completed":
      return "ok";
    case "running":
    case "starting":
      return "accent";
    case "queued":
    case "cancelling":
      return "warn";
    case "failed":
    case "timeout":
      return "danger";
    default:
      return "muted";
  }
}

export function botTone(state: string | undefined): Tone {
  switch (state) {
    case "polling":
      return "ok";
    case "paused":
    case "starting":
    case "stopping":
      return "warn";
    default:
      return "muted";
  }
}

export const botLabel: Record<string, string> = {
  polling: "Running",
  paused: "Paused",
  starting: "Starting…",
  stopping: "Stopping…",
  stopped: "Stopped",
};

export function dataURL(b64: string | undefined | null, type = "image/jpeg"): string {
  return b64 ? `data:${type};base64,${b64}` : "";
}

// validNewID checks an ID typed into the allowlist editor.
export function idProblem(v: string, existing: number[]): string {
  v = v.trim();
  if (!v) return "";
  if (!/^[1-9][0-9]{4,15}$/.test(v)) return "A Telegram user ID is a positive number.";
  if (existing.some((x) => String(x) === v)) return "Already on the list.";
  return "";
}
