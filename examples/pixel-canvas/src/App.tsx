import { useCallback, useEffect, useState } from "react";
import Canvas, { type PixelGrid } from "./components/Canvas";
import ColorPalette from "./components/ColorPalette";
import CooldownTimer, { remainingMs } from "./components/CooldownTimer";
import AuthModal from "./components/AuthModal";
import Header from "./components/Header";
import PixelInfo from "./components/PixelInfo";
import { useRealtime, type RealtimeEvent } from "./hooks/useRealtime";
import { ayb, isLoggedIn } from "./lib/ayb";
import { GRID_SIZE } from "./constants";

function createEmptyGrid(): PixelGrid {
  const cells = new Int8Array(GRID_SIZE * GRID_SIZE);
  cells.fill(-1);
  return { cells };
}

export default function App() {
  const [grid, setGrid] = useState<PixelGrid>(createEmptyGrid);
  const [selectedColor, setSelectedColor] = useState(5); // red
  const [lastPlacedAt, setLastPlacedAt] = useState(0);
  const [showAuth, setShowAuth] = useState(false);
  const [user, setUser] = useState<string | null>(null);
  const [pixelCount, setPixelCount] = useState(0);
  const [hover, setHover] = useState<{ x: number; y: number } | null>(null);

  // Load current user on mount.
  useEffect(() => {
    if (isLoggedIn()) {
      ayb.auth.me().then((u) => setUser(u.email)).catch(() => {});
    }
  }, []);

  // Load initial canvas state.
  useEffect(() => {
    async function load() {
      try {
        const res = await ayb.records.list<{
          x: number;
          y: number;
          color: number;
        }>("pixels", { perPage: 10000, skipTotal: true });
        const g = createEmptyGrid();
        let count = 0;
        for (const p of res.items) {
          if (p.x >= 0 && p.x < GRID_SIZE && p.y >= 0 && p.y < GRID_SIZE) {
            g.cells[p.y * GRID_SIZE + p.x] = p.color;
            count++;
          }
        }
        setGrid(g);
        setPixelCount(count);
      } catch {
        // Canvas starts empty if server unavailable.
      }
    }
    load();
  }, []);

  // Realtime: update grid when other users place pixels.
  const handleRealtimeEvent = useCallback((event: RealtimeEvent) => {
    if (event.table !== "pixels") return;
    const rec = event.record as { x: number; y: number; color: number };
    if (
      rec.x == null ||
      rec.y == null ||
      rec.x < 0 || rec.x >= GRID_SIZE ||
      rec.y < 0 || rec.y >= GRID_SIZE
    ) return;

    setGrid((prev) => {
      const next = { cells: new Int8Array(prev.cells) };
      const wasEmpty = prev.cells[rec.y * GRID_SIZE + rec.x] < 0;
      next.cells[rec.y * GRID_SIZE + rec.x] = rec.color;
      if (wasEmpty) {
        setPixelCount((c) => c + 1);
      }
      return next;
    });
  }, []);

  useRealtime(["pixels"], handleRealtimeEvent);

  // Place a pixel.
  const handlePlace = useCallback(
    async (x: number, y: number) => {
      if (!isLoggedIn()) {
        setShowAuth(true);
        return;
      }
      if (remainingMs(lastPlacedAt, Date.now()) > 0) return;

      // Optimistic update.
      setGrid((prev) => {
        const next = { cells: new Int8Array(prev.cells) };
        const wasEmpty = prev.cells[y * GRID_SIZE + x] < 0;
        next.cells[y * GRID_SIZE + x] = selectedColor;
        if (wasEmpty) setPixelCount((c) => c + 1);
        return next;
      });
      setLastPlacedAt(Date.now());

      try {
        // Use RPC for atomic upsert.
        const resp = await fetch("/api/rpc/place_pixel", {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
            Authorization: `Bearer ${ayb.token}`,
          },
          body: JSON.stringify({ px: x, py: y, pcolor: selectedColor }),
        });
        if (!resp.ok) {
          // Revert on failure.
          setGrid((prev) => {
            const next = { cells: new Int8Array(prev.cells) };
            next.cells[y * GRID_SIZE + x] = -1;
            return next;
          });
          setLastPlacedAt(0);
        }
      } catch {
        // Revert on network error.
        setGrid((prev) => {
          const next = { cells: new Int8Array(prev.cells) };
          next.cells[y * GRID_SIZE + x] = -1;
          return next;
        });
        setLastPlacedAt(0);
      }
    },
    [selectedColor, lastPlacedAt],
  );

  const canPlace =
    isLoggedIn() && remainingMs(lastPlacedAt, Date.now()) <= 0;

  const handleHover = useCallback(
    (coords: { x: number; y: number } | null) => setHover(coords),
    [],
  );

  return (
    <>
      <Canvas
        grid={grid}
        selectedColor={selectedColor}
        canPlace={canPlace}
        onPlace={handlePlace}
        onHover={handleHover}
      />
      <Header
        pixelCount={pixelCount}
        user={user}
        onLoginClick={() => setShowAuth(true)}
        onLogout={() => setUser(null)}
      />
      <div className="fixed bottom-4 left-1/2 -translate-x-1/2 z-30 flex items-center gap-3">
        <ColorPalette
          selected={selectedColor}
          onSelect={setSelectedColor}
          disabled={!isLoggedIn()}
        />
        <CooldownTimer lastPlacedAt={lastPlacedAt} />
        <PixelInfo coords={hover} />
      </div>
      {showAuth && (
        <AuthModal
          onClose={() => setShowAuth(false)}
          onAuth={() => {
            ayb.auth.me().then((u) => setUser(u.email)).catch(() => {});
          }}
        />
      )}
    </>
  );
}
