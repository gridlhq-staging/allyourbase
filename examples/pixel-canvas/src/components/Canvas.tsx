import { useCallback, useEffect, useRef, useState } from "react";
import { GRID_SIZE, PALETTE } from "../constants";

/** Minimum and maximum zoom level (pixels per cell). */
const MIN_ZOOM = 2;
const MAX_ZOOM = 40;
const DEFAULT_ZOOM = 6;

export interface PixelGrid {
  /** color index per cell, length = GRID_SIZE * GRID_SIZE. -1 = empty. */
  cells: Int8Array;
}

interface Props {
  grid: PixelGrid;
  selectedColor: number;
  canPlace: boolean;
  onPlace: (x: number, y: number) => void;
  onHover: (coords: { x: number; y: number } | null) => void;
}

/** Convert screen coordinates to grid cell. Returns null if out of bounds. */
export function screenToGrid(
  sx: number,
  sy: number,
  zoom: number,
  offsetX: number,
  offsetY: number,
): { x: number; y: number } | null {
  const gx = Math.floor((sx - offsetX) / zoom);
  const gy = Math.floor((sy - offsetY) / zoom);
  if (gx < 0 || gy < 0 || gx >= GRID_SIZE || gy >= GRID_SIZE) return null;
  return { x: gx, y: gy };
}

export default function Canvas({ grid, selectedColor, canPlace, onPlace, onHover }: Props) {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const [zoom, setZoom] = useState(DEFAULT_ZOOM);
  const [offset, setOffset] = useState({ x: 0, y: 0 });
  const [hover, setHover] = useState<{ x: number; y: number } | null>(null);
  const dragging = useRef(false);
  const dragStart = useRef({ x: 0, y: 0, ox: 0, oy: 0 });

  // Center the grid on mount.
  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const rect = canvas.getBoundingClientRect();
    setOffset({
      x: Math.floor((rect.width - GRID_SIZE * DEFAULT_ZOOM) / 2),
      y: Math.floor((rect.height - GRID_SIZE * DEFAULT_ZOOM) / 2),
    });
  }, []);

  // Resize canvas to fill viewport.
  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const resize = () => {
      canvas.width = window.innerWidth;
      canvas.height = window.innerHeight;
    };
    resize();
    window.addEventListener("resize", resize);
    return () => window.removeEventListener("resize", resize);
  }, []);

  // Draw loop.
  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const ctx = canvas.getContext("2d");
    if (!ctx) return;

    let raf = 0;
    const draw = () => {
      ctx.fillStyle = "#111";
      ctx.fillRect(0, 0, canvas.width, canvas.height);

      // Draw grid background (empty = dark).
      ctx.fillStyle = "#1a1a2e";
      ctx.fillRect(offset.x, offset.y, GRID_SIZE * zoom, GRID_SIZE * zoom);

      // Draw pixels.
      for (let y = 0; y < GRID_SIZE; y++) {
        for (let x = 0; x < GRID_SIZE; x++) {
          const color = grid.cells[y * GRID_SIZE + x];
          if (color >= 0) {
            ctx.fillStyle = PALETTE[color];
            ctx.fillRect(
              offset.x + x * zoom,
              offset.y + y * zoom,
              zoom,
              zoom,
            );
          }
        }
      }

      // Grid lines when zoomed in enough.
      if (zoom >= 8) {
        ctx.strokeStyle = "rgba(255,255,255,0.08)";
        ctx.lineWidth = 1;
        for (let i = 0; i <= GRID_SIZE; i++) {
          const px = offset.x + i * zoom;
          const py = offset.y + i * zoom;
          ctx.beginPath();
          ctx.moveTo(px, offset.y);
          ctx.lineTo(px, offset.y + GRID_SIZE * zoom);
          ctx.stroke();
          ctx.beginPath();
          ctx.moveTo(offset.x, py);
          ctx.lineTo(offset.x + GRID_SIZE * zoom, py);
          ctx.stroke();
        }
      }

      // Hover highlight.
      if (hover) {
        ctx.strokeStyle = canPlace ? "#fff" : "rgba(255,255,255,0.3)";
        ctx.lineWidth = 2;
        ctx.strokeRect(
          offset.x + hover.x * zoom,
          offset.y + hover.y * zoom,
          zoom,
          zoom,
        );
        if (canPlace) {
          ctx.fillStyle = PALETTE[selectedColor] + "80";
          ctx.fillRect(
            offset.x + hover.x * zoom,
            offset.y + hover.y * zoom,
            zoom,
            zoom,
          );
        }
      }

      raf = requestAnimationFrame(draw);
    };
    raf = requestAnimationFrame(draw);
    return () => cancelAnimationFrame(raf);
  }, [grid, zoom, offset, hover, selectedColor, canPlace]);

  const handleWheel = useCallback(
    (e: React.WheelEvent) => {
      e.preventDefault();
      const rect = canvasRef.current!.getBoundingClientRect();
      const mx = e.clientX - rect.left;
      const my = e.clientY - rect.top;
      const oldZoom = zoom;
      const newZoom = Math.max(
        MIN_ZOOM,
        Math.min(MAX_ZOOM, zoom - Math.sign(e.deltaY) * Math.max(1, Math.floor(zoom / 5))),
      );
      // Zoom toward cursor.
      const scale = newZoom / oldZoom;
      setOffset({
        x: Math.round(mx - (mx - offset.x) * scale),
        y: Math.round(my - (my - offset.y) * scale),
      });
      setZoom(newZoom);
    },
    [zoom, offset],
  );

  const handleMouseDown = useCallback(
    (e: React.MouseEvent) => {
      dragging.current = true;
      dragStart.current = { x: e.clientX, y: e.clientY, ox: offset.x, oy: offset.y };
    },
    [offset],
  );

  const handleMouseMove = useCallback(
    (e: React.MouseEvent) => {
      const rect = canvasRef.current!.getBoundingClientRect();
      const cell = screenToGrid(
        e.clientX - rect.left,
        e.clientY - rect.top,
        zoom,
        offset.x,
        offset.y,
      );
      setHover(cell);
      onHover(cell);

      if (dragging.current) {
        const dx = e.clientX - dragStart.current.x;
        const dy = e.clientY - dragStart.current.y;
        setOffset({
          x: dragStart.current.ox + dx,
          y: dragStart.current.oy + dy,
        });
      }
    },
    [zoom, offset, onHover],
  );

  const handleMouseUp = useCallback(
    (e: React.MouseEvent) => {
      const wasDrag =
        Math.abs(e.clientX - dragStart.current.x) > 3 ||
        Math.abs(e.clientY - dragStart.current.y) > 3;
      dragging.current = false;
      if (wasDrag) return;

      // Click (not drag) — place pixel.
      if (!canPlace) return;
      const rect = canvasRef.current!.getBoundingClientRect();
      const cell = screenToGrid(
        e.clientX - rect.left,
        e.clientY - rect.top,
        zoom,
        offset.x,
        offset.y,
      );
      if (cell) onPlace(cell.x, cell.y);
    },
    [zoom, offset, canPlace, onPlace],
  );

  const handleMouseLeave = useCallback(() => {
    setHover(null);
    onHover(null);
    dragging.current = false;
  }, [onHover]);

  return (
    <canvas
      ref={canvasRef}
      className="fixed inset-0 cursor-crosshair"
      onWheel={handleWheel}
      onMouseDown={handleMouseDown}
      onMouseMove={handleMouseMove}
      onMouseUp={handleMouseUp}
      onMouseLeave={handleMouseLeave}
    />
  );
}
