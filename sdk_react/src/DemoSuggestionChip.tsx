import type { DemoSuggestion } from "./types";

interface DemoSuggestionChipProps {
  suggestion: DemoSuggestion;
  onSelect: (suggestion: DemoSuggestion) => void;
}

export function DemoSuggestionChip({ suggestion, onSelect }: DemoSuggestionChipProps) {
  return (
    <button type="button" onClick={() => onSelect(suggestion)}>
      {suggestion.label}
    </button>
  );
}
