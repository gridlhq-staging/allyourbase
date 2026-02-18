/** Canvas grid dimensions. */
export const GRID_SIZE = 100;

/** Cooldown between pixel placements in milliseconds. */
export const COOLDOWN_MS = 5_000;

/** 16-color palette (classic r/place). Index is the color value stored in DB. */
export const PALETTE: readonly string[] = [
  "#FFFFFF", // 0  white
  "#E4E4E4", // 1  light gray
  "#888888", // 2  gray
  "#222222", // 3  dark gray
  "#FFA7D1", // 4  pink
  "#E50000", // 5  red
  "#E59500", // 6  orange
  "#A06A42", // 7  brown
  "#E5D900", // 8  yellow
  "#94E044", // 9  lime
  "#02BE01", // 10 green
  "#00D3DD", // 11 cyan
  "#0083C7", // 12 blue
  "#0000EA", // 13 dark blue
  "#CF6EE4", // 14 purple
  "#820080", // 15 dark purple
] as const;

/** Keyboard shortcuts for color selection: keys 1-9, 0, a-f map to palette indices 0-15. */
export const COLOR_KEYS: Record<string, number> = {
  "1": 0, "2": 1, "3": 2, "4": 3, "5": 4,
  "6": 5, "7": 6, "8": 7, "9": 8, "0": 9,
  a: 10, b: 11, c: 12, d: 13, e: 14, f: 15,
};
