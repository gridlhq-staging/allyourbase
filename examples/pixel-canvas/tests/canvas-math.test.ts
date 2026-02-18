import { describe, it, expect } from "vitest";
import { screenToGrid } from "../src/components/Canvas";
import { GRID_SIZE } from "../src/constants";

describe("screenToGrid", () => {
  const zoom = 10;
  const offsetX = 50;
  const offsetY = 50;

  it("maps screen coords to grid cell", () => {
    // Click at screen (55, 65) => grid offset is (50,50), zoom=10
    // gx = floor((55-50)/10) = 0, gy = floor((65-50)/10) = 1
    const result = screenToGrid(55, 65, zoom, offsetX, offsetY);
    expect(result).toEqual({ x: 0, y: 1 });
  });

  it("returns last valid cell for bottom-right pixel", () => {
    // Bottom-right of the grid: gx = 99, gy = 99
    const sx = offsetX + 99 * zoom + 5; // middle of cell (99, 99)
    const sy = offsetY + 99 * zoom + 5;
    const result = screenToGrid(sx, sy, zoom, offsetX, offsetY);
    expect(result).toEqual({ x: 99, y: 99 });
  });

  it("returns null for coords before the grid", () => {
    expect(screenToGrid(40, 60, zoom, offsetX, offsetY)).toBeNull();
    expect(screenToGrid(60, 40, zoom, offsetX, offsetY)).toBeNull();
  });

  it("returns null for coords beyond the grid", () => {
    const beyondX = offsetX + GRID_SIZE * zoom + 1;
    expect(screenToGrid(beyondX, 60, zoom, offsetX, offsetY)).toBeNull();
  });

  it("handles zoom=1 correctly", () => {
    const result = screenToGrid(5, 5, 1, 0, 0);
    expect(result).toEqual({ x: 5, y: 5 });
  });

  it("handles large zoom correctly", () => {
    const result = screenToGrid(200, 200, 40, 0, 0);
    expect(result).toEqual({ x: 5, y: 5 });
  });

  it("handles negative offsets (panned left/up past origin)", () => {
    const result = screenToGrid(10, 10, 10, -100, -100);
    // gx = floor((10 - (-100)) / 10) = floor(110/10) = 11
    expect(result).toEqual({ x: 11, y: 11 });
  });
});
