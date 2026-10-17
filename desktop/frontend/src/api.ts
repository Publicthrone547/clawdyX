// Typed access to the Go services. All calls go to the Go side, which talks
// to the worker over its private socket; the UI never runs commands.
export * as Worker from "../bindings/github.com/Publicthrone547/clawdyX/desktop/workerservice.js";
export * as System from "../bindings/github.com/Publicthrone547/clawdyX/desktop/systemservice.js";
export * as Setup from "../bindings/github.com/Publicthrone547/clawdyX/desktop/setupservice.js";
export type * from "../bindings/github.com/Publicthrone547/clawdyX/internal/control/models.js";
export type * as D from "../bindings/github.com/Publicthrone547/clawdyX/desktop/models.js";
export type { Sender } from "../bindings/github.com/Publicthrone547/clawdyX/internal/setup/models.js";

export function errMsg(e: unknown): string {
  if (e instanceof Error) return e.message;
  if (typeof e === "string") return e;
  try {
    const m = (e as { message?: string })?.message;
    if (m) return m;
  } catch {
    /* ignore */
  }
  return String(e);
}
