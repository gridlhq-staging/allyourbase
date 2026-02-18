import { describe, it, expect } from "vitest";
import { PALETTE, COLOR_KEYS, GRID_SIZE, COOLDOWN_MS } from "../src/constants";

describe("constants", () => {
  it("PALETTE has exactly 16 colors", () => {
    expect(PALETTE).toHaveLength(16);
  });

  it("all PALETTE entries are valid hex colors", () => {
    for (const hex of PALETTE) {
      expect(hex).toMatch(/^#[0-9A-F]{6}$/);
    }
  });

  it("COLOR_KEYS maps 16 keys to palette indices 0-15", () => {
    const values = Object.values(COLOR_KEYS).sort((a, b) => a - b);
    expect(values).toEqual(Array.from({ length: 16 }, (_, i) => i));
  });

  it("GRID_SIZE is a positive integer", () => {
    expect(GRID_SIZE).toBeGreaterThan(0);
    expect(Number.isInteger(GRID_SIZE)).toBe(true);
  });

  it("COOLDOWN_MS is a positive number", () => {
    expect(COOLDOWN_MS).toBeGreaterThan(0);
  });
});
