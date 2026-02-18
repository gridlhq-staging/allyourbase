interface Props {
  coords: { x: number; y: number } | null;
}

export default function PixelInfo({ coords }: Props) {
  if (!coords) return null;
  return (
    <div className="px-2 py-1 bg-gray-900/90 rounded-lg backdrop-blur-sm text-xs text-gray-400 tabular-nums">
      ({coords.x}, {coords.y})
    </div>
  );
}
