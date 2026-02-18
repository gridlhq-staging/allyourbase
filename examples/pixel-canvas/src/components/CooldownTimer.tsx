import { useEffect, useState } from "react";
import { COOLDOWN_MS } from "../constants";

interface Props {
  /** Timestamp (ms) of the last pixel placement, or 0 if none. */
  lastPlacedAt: number;
}

/**
 * Returns remaining cooldown in milliseconds, clamped to [0, COOLDOWN_MS].
 */
export function remainingMs(lastPlacedAt: number, now: number): number {
  if (lastPlacedAt === 0) return 0;
  return Math.max(0, COOLDOWN_MS - (now - lastPlacedAt));
}

export default function CooldownTimer({ lastPlacedAt }: Props) {
  const [now, setNow] = useState(Date.now());

  useEffect(() => {
    if (lastPlacedAt === 0) return;
    const id = setInterval(() => setNow(Date.now()), 100);
    return () => clearInterval(id);
  }, [lastPlacedAt]);

  const ms = remainingMs(lastPlacedAt, now);
  if (ms <= 0) return null;

  const secs = Math.ceil(ms / 1000);
  const pct = (ms / COOLDOWN_MS) * 100;

  return (
    <div className="flex items-center gap-2 px-3 py-1.5 bg-gray-900/90 rounded-lg backdrop-blur-sm text-sm">
      <div className="w-20 h-2 bg-gray-700 rounded-full overflow-hidden">
        <div
          className="h-full bg-blue-500 rounded-full transition-all"
          style={{ width: `${pct}%` }}
        />
      </div>
      <span className="text-gray-300 tabular-nums">{secs}s</span>
    </div>
  );
}
