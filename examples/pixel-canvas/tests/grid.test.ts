import { describe, it, expect } from "vitest";
import { GRID_SIZE, PALETTE } from "../src/constants";

describe("PixelGrid data structure", () => {
  function createEmptyGrid(): Int8Array {
    const cells = new Int8Array(GRID_SIZE * GRID_SIZE);
    cells.fill(-1);
    return cells;
  }

  it("empty grid has all cells set to -1", () => {
    const cells = createEmptyGrid();
    for (let i = 0; i < cells.length; i++) {
      expect(cells[i]).toBe(-1);
    }
  });

  it("setting a pixel updates the correct cell", () => {
    const cells = createEmptyGrid();
    const x = 42;
    const y = 73;
    const color = 5;
    cells[y * GRID_SIZE + x] = color;
    expect(cells[y * GRID_SIZE + x]).toBe(color);
    // Neighbors are unchanged.
    expect(cells[y * GRID_SIZE + (x + 1)]).toBe(-1);
    expect(cells[(y + 1) * GRID_SIZE + x]).toBe(-1);
  });

  it("grid size matches total cell count", () => {
    const cells = createEmptyGrid();
    expect(cells.length).toBe(GRID_SIZE * GRID_SIZE);
  });

  it("all palette colors are valid grid values", () => {
    const cells = createEmptyGrid();
    for (let i = 0; i < PALETTE.length; i++) {
      cells[i] = i;
      expect(cells[i]).toBe(i);
    }
  });

  it("pixel count matches set pixels", () => {
    const cells = createEmptyGrid();
    const positions = [
      [0, 0],
      [50, 50],
      [99, 99],
    ];
    for (const [x, y] of positions) {
      cells[y * GRID_SIZE + x] = 0;
    }
    let count = 0;
    for (let i = 0; i < cells.length; i++) {
      if (cells[i] >= 0) count++;
    }
    expect(count).toBe(3);
  });

  it("overwriting a pixel preserves other pixels", () => {
    const cells = createEmptyGrid();
    cells[0] = 5;
    cells[1] = 10;
    cells[0] = 3; // overwrite
    expect(cells[0]).toBe(3);
    expect(cells[1]).toBe(10);
  });
});
