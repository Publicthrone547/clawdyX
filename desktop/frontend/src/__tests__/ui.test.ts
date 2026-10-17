import { describe, expect, it } from "vitest";
import { mount } from "@vue/test-utils";
import SecretInput from "../components/SecretInput.vue";
import Badge from "../components/Badge.vue";
import { ago, bytes, dur, idProblem, jobTone, botLabel } from "../util";

describe("SecretInput", () => {
  it("masks the token until the user asks to show it", async () => {
    const w = mount(SecretInput, { props: { id: "t", modelValue: "123456789:SECRET" } });
    const input = w.get("input");
    expect(input.attributes("type")).toBe("password");
    expect(input.attributes("autocomplete")).toBe("off");
    await w.get("button").trigger("click");
    expect(w.get("input").attributes("type")).toBe("text");
    expect(w.get("button").attributes("aria-pressed")).toBe("true");
  });
  it("emits typed text", async () => {
    const w = mount(SecretInput, { props: { id: "t", modelValue: "" } });
    await w.get("input").setValue("abc");
    expect(w.emitted("update:modelValue")?.[0]).toEqual(["abc"]);
  });
});

describe("Badge", () => {
  it("always shows a text label, not just a colour", () => {
    const w = mount(Badge, { props: { tone: "danger", label: "Not running" } });
    expect(w.text()).toContain("Not running");
  });
});

describe("allowlist ID validation", () => {
  it("accepts positive numeric IDs only", () => {
    expect(idProblem("123456789", [])).toBe("");
    expect(idProblem("", [])).toBe("");
    for (const bad of ["0", "-5", "12ab", "@someone", "1.5", "123"]) expect(idProblem(bad, [])).not.toBe("");
  });
  it("rejects duplicates", () => {
    expect(idProblem("111111111", [111111111])).toContain("Already");
  });
});

describe("formatting", () => {
  it("formats times, durations and sizes", () => {
    expect(ago(0)).toBe("never");
    expect(ago(Date.now() / 1000 - 120)).toBe("2 min ago");
    expect(dur(65_000)).toBe("1 min 5 s");
    expect(bytes(2048)).toBe("2 KB");
    expect(jobTone("failed")).toBe("danger");
    expect(botLabel.polling).toBe("Running");
  });
});
