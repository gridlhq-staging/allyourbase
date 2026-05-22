/**
 * @module Idempotent sample-board seeding for the Kanban demo.
 *
 * A fresh anonymous user (delivered by the anonymous-first entry flow) should
 * land on a populated board rather than an empty shell. `ensureSampleBoard()`
 * creates exactly one starter board — with starter columns and cards — but
 * only when the current user owns zero boards.
 *
 * Idempotency keys on USER-OWNED boards (`user_id === me.id`), never on the
 * global board list. `boards_select USING (true)` in schema.sql makes every
 * board globally visible, so `records.list("boards")` returns all users'
 * boards; keying on the global length would seed only the very first user and
 * never anyone after them.
 */
import { ayb } from "./ayb";
import type { Board, Card, Column } from "../types";

/** Starter column titles in display order; the array index is the `position`. */
const STARTER_COLUMNS = ["To Do", "In Progress", "Done"] as const;

/** Starter cards, each parented to a starter column by its array index. */
const STARTER_CARDS: ReadonlyArray<{
  columnIndex: number;
  title: string;
  description: string;
}> = [
  {
    columnIndex: 0,
    title: "Welcome to your board",
    description: "Click any card to edit its title and description.",
  },
  {
    columnIndex: 0,
    title: "Drag cards between columns",
    description: "Try moving this card into In Progress.",
  },
  {
    columnIndex: 1,
    title: "Invite a teammate",
    description: "Boards update in real time for everyone viewing them.",
  },
  {
    columnIndex: 2,
    title: "Ship something",
    description: "Drop finished work here to mark it done.",
  },
];

/**
 * In-flight guard: repeated mounts — including React 18 StrictMode's
 * double-invocation — await the same seed run instead of racing two passes.
 * Cleared once settled so a later mount (e.g. a different user after logout)
 * re-runs the idempotency check; this is a concurrency guard, not a result
 * cache.
 */
let seedInFlight: Promise<void> | null = null;

/**
 * localStorage key holding the user id of a seed run that has started but not
 * been confirmed complete. Set before the very first board create, cleared
 * once seeding finishes fully.
 *
 * Its presence is the ONLY proof that an owned "My First Board" with missing
 * columns is an interrupted seed run rather than a board the user edited down
 * themselves (the demo lets users delete columns). The repair path must never
 * delete a board without this proof — doing so is silent user data loss.
 */
const SEED_IN_PROGRESS_KEY = "ayb_kanban_seed_in_progress";

/** Record that a seed run for `userId` has started but not yet completed. */
function markSeedInProgress(userId: string): void {
  try {
    localStorage.setItem(SEED_IN_PROGRESS_KEY, userId);
  } catch {
    // localStorage unavailable — the repair path simply won't fire, which is
    // the safe default: never delete a board we cannot prove is a partial seed.
  }
}

/** Clear the in-progress marker once a seed run is confirmed complete. */
function clearSeedInProgress(): void {
  try {
    localStorage.removeItem(SEED_IN_PROGRESS_KEY);
  } catch {
    // Ignore — see markSeedInProgress for why a missing marker is safe.
  }
}

/** Whether a seed run for `userId` is recorded as started-but-not-completed. */
function seedInProgressFor(userId: string): boolean {
  try {
    return localStorage.getItem(SEED_IN_PROGRESS_KEY) === userId;
  } catch {
    return false;
  }
}

/** Create one starter board owned by `userId`, with starter columns and cards. */
async function createStarterBoard(userId: string): Promise<void> {
  // Mark the run in-progress BEFORE the board exists, so an interrupted run
  // (tab closed mid-seed, a failed rollback) leaves proof that the resulting
  // partial board came from seeding — the only signal the repair path trusts.
  markSeedInProgress(userId);

  let board: Board;
  try {
    board = await ayb.records.create<Board>("boards", {
      title: "My First Board",
      user_id: userId,
    });
  } catch (err) {
    // The board was never created — clear the marker so it cannot later be
    // mistaken as proof of an interrupted seed. A stale marker with no
    // partial board would let the repair path delete a legitimate
    // user-created "My First Board" that simply has fewer than three columns.
    clearSeedInProgress();
    throw err;
  }

  try {
    const columns: Column[] = [];
    for (let position = 0; position < STARTER_COLUMNS.length; position++) {
      const column = await ayb.records.create<Column>("columns", {
        board_id: board.id,
        title: STARTER_COLUMNS[position],
        position,
      });
      columns.push(column);
    }

    const cardCountByColumn = new Array<number>(columns.length).fill(0);
    for (const card of STARTER_CARDS) {
      const column = columns[card.columnIndex];
      await ayb.records.create<Card>("cards", {
        column_id: column.id,
        title: card.title,
        description: card.description,
        position: cardCountByColumn[card.columnIndex]++,
      });
    }
  } catch (err) {
    // Roll back the board so a subsequent retry can start fresh.
    try {
      await ayb.records.delete("boards", board.id);
      // Rollback succeeded — no partial board remains, so the in-progress
      // marker is now stale. Clear it: a leftover marker plus a later
      // legitimate "My First Board" with fewer than three columns could be
      // misread by the repair path as an interrupted seed and deleted.
      clearSeedInProgress();
    } catch {
      // Rollback failed — the partial board still exists. KEEP the marker so
      // a later repair pass can prove that board is an interrupted seed run.
    }
    throw err;
  }

  // Full success — the board now has all starter content; clear the marker.
  clearSeedInProgress();
}

/** Check whether an owned starter board has its expected columns and cards. */
async function isStarterBoardComplete(boardId: string): Promise<boolean> {
  const colRes = await ayb.records.list<Column>("columns", {
    filter: `board_id='${boardId}'`,
    perPage: 100,
  });
  if (colRes.items.length < STARTER_COLUMNS.length) return false;

  const columnIdByTitle = new Map<string, string>();
  for (const columnTitle of STARTER_COLUMNS) {
    const column = colRes.items.find((item) => item.title === columnTitle);
    if (!column) return false;
    columnIdByTitle.set(columnTitle, column.id);
  }

  for (let columnIndex = 0; columnIndex < STARTER_COLUMNS.length; columnIndex++) {
    const columnTitle = STARTER_COLUMNS[columnIndex];
    const columnId = columnIdByTitle.get(columnTitle);
    if (!columnId) return false;

    const expectedTitles = STARTER_CARDS
      .filter((card) => card.columnIndex === columnIndex)
      .map((card) => card.title);
    if (expectedTitles.length === 0) continue;

    const cardRes = await ayb.records.list<Card>("cards", {
      filter: `column_id='${columnId}'`,
      perPage: 100,
    });
    const foundTitles = new Set(cardRes.items.map((card) => card.title));
    if (expectedTitles.some((title) => !foundTitles.has(title))) return false;
  }

  return true;
}

/** Seed a starter board for the current user only if they own none. */
async function seedIfNeeded(): Promise<void> {
  const me = await ayb.auth.me();

  // Server-side `user_id` filter narrows the globally-visible board list to
  // the current user (the same `filter` mechanism BoardView already relies
  // on). The client-side re-filter is a defensive second check in case the
  // filter param is ever ignored — without it a missed filter could make a
  // returning user look board-less and trigger a duplicate seed.
  const res = await ayb.records.list<Board>("boards", {
    filter: `user_id='${me.id}'`,
    perPage: 100,
  });
  const ownedBoards = res.items.filter((board) => board.user_id === me.id);

  if (ownedBoards.length === 0) {
    await createStarterBoard(me.id);
    return;
  }

  // Repair path: re-seed a partially-seeded starter board (missing columns).
  //
  // This deletes a board, so it must only fire on state PROVEN to be an
  // interrupted seed run. Title + column count alone is not proof — a user
  // can name a board "My First Board" and delete columns down below three.
  // The `seedInProgressFor` marker is the proof: it is set before seeding
  // starts and cleared only on full success, so a board missing columns
  // WHILE the marker is still set can only be an interrupted seed.
  if (
    ownedBoards.length === 1 &&
    ownedBoards[0].title === "My First Board" &&
    seedInProgressFor(me.id)
  ) {
    const complete = await isStarterBoardComplete(ownedBoards[0].id);
    if (!complete) {
      // Preserve the "exactly one starter board" invariant: if the partial
      // board cannot be removed, abort repair rather than creating a second
      // starter board beside the still-existing incomplete one.
      await ayb.records.delete("boards", ownedBoards[0].id);
      await createStarterBoard(me.id);
    } else {
      // The board is actually complete — the run finished but the marker was
      // never cleared (e.g. tab closed right after the last create). Clear
      // the stale marker so a later user edit can't be misread as a partial.
      clearSeedInProgress();
    }
  }
}

/**
 * Seed a starter board for the current user if they own none.
 *
 * Safe to call on every authenticated mount: it no-ops once the user owns at
 * least one board. Creation errors reject so the caller can decide how to
 * surface them.
 */
export async function ensureSampleBoard(): Promise<void> {
  if (seedInFlight) return seedInFlight;

  seedInFlight = seedIfNeeded();
  try {
    await seedInFlight;
  } finally {
    // Settle the guard so the next mount re-checks idempotency afresh.
    seedInFlight = null;
  }
}
