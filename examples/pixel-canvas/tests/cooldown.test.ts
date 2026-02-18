import { describe, it, expect } from "vitest";
import { remainingMs } from "../src/components/CooldownTimer";
import { COOLDOWN_MS } from "../src/constants";

describe("remainingMs", () => {
  it("returns 0 when lastPlacedAt is 0 (never placed)", () => {
    expect(remainingMs(0, Date.now())).toBe(0);
  });

  it("returns full cooldown immediately after placement", () => {
    const now = 1000000;
    expect(remainingMs(now, now)).toBe(COOLDOWN_MS);
  });

  it("returns half cooldown at midpoint", () => {
    const now = 1000000;
    const half = now + COOLDOWN_MS / 2;
    expect(remainingMs(now, half)).toBe(COOLDOWN_MS / 2);
  });

  it("returns 0 after cooldown expires", () => {
    const now = 1000000;
    const after = now + COOLDOWN_MS + 1;
    expect(remainingMs(now, after)).toBe(0);
  });

  it("never returns negative", () => {
    const now = 1000000;
    const wayAfter = now + COOLDOWN_MS * 10;
    expect(remainingMs(now, wayAfter)).toBe(0);
  });
});
