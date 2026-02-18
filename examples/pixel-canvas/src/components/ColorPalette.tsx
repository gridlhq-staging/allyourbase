import { useEffect } from "react";
import { PALETTE, COLOR_KEYS } from "../constants";

interface Props {
  selected: number;
  onSelect: (index: number) => void;
  disabled: boolean;
}

export default function ColorPalette({ selected, onSelect, disabled }: Props) {
  // Keyboard shortcuts.
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.target instanceof HTMLInputElement) return;
      const idx = COLOR_KEYS[e.key.toLowerCase()];
      if (idx !== undefined) onSelect(idx);
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [onSelect]);

  return (
    <div className="flex gap-1 p-2 bg-gray-900/90 rounded-lg backdrop-blur-sm">
      {PALETTE.map((hex, i) => (
        <button
          key={i}
          title={`Color ${i} (key: ${Object.entries(COLOR_KEYS).find(([, v]) => v === i)?.[0]})`}
          disabled={disabled}
          onClick={() => onSelect(i)}
          className={`w-7 h-7 rounded border-2 transition-transform ${
            i === selected
              ? "border-white scale-125 z-10"
              : "border-gray-700 hover:border-gray-400"
          } ${disabled ? "opacity-40 cursor-not-allowed" : "cursor-pointer"}`}
          style={{ backgroundColor: hex }}
        />
      ))}
    </div>
  );
}
