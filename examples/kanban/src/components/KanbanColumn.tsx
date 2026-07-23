import { useState } from "react";
import { Droppable } from "@hello-pangea/dnd";
import { ayb } from "../lib/ayb";
import type { Card, Column } from "../types";
import KanbanCard from "./KanbanCard";

interface Props {
  column: Column;
  cards: Card[];
  onCardClick: (card: Card) => void;
  onCardCreated: (card: Card) => void;
  onDeleteColumn: (columnId: string) => void;
}

export default function KanbanColumn({
  column,
  cards,
  onCardClick,
  onCardCreated,
  onDeleteColumn,
}: Props) {
  const [newTitle, setNewTitle] = useState("");
  const [adding, setAdding] = useState(false);
  const [showAdd, setShowAdd] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function addCard(e: React.FormEvent) {
    e.preventDefault();
    if (!newTitle.trim()) return;
    setAdding(true);
    setError(null);
    try {
      const card = await ayb.records.create<Card>("cards", {
        column_id: column.id,
        title: newTitle.trim(),
        position: cards.length,
      });
      onCardCreated(card);
      setNewTitle("");
      setShowAdd(false);
    } catch (err) {
      console.error("Failed to create card:", err);
      setError(err instanceof Error ? err.message : "Failed to create card");
    } finally {
      setAdding(false);
    }
  }

  async function handleDeleteColumn() {
    if (!confirm(`Delete "${column.title}" and all its cards?`)) return;
    setError(null);
    try {
      await ayb.records.delete("columns", column.id);
      onDeleteColumn(column.id);
    } catch (err) {
      console.error("Failed to delete column:", err);
      setError(err instanceof Error ? err.message : "Failed to delete column");
    }
  }

  return (
    <div data-testid={`column-${column.title}`} className="bg-gray-100 rounded-xl p-3 w-72 flex-shrink-0 flex flex-col max-h-[calc(100vh-10rem)]">
      <div className="flex items-center justify-between mb-3 px-1">
        <h3 className="font-semibold text-sm text-gray-700 uppercase tracking-wide">
          {column.title}
          <span data-testid="card-count" className="ml-2 text-gray-500 font-normal">{cards.length}</span>
        </h3>
        <button
          onClick={handleDeleteColumn}
          aria-label={`Delete column ${column.title}`}
          className="text-gray-500 hover:text-red-700 transition-colors"
          title="Delete column"
        >
          <svg
            className="w-4 h-4"
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeWidth={2}
              d="M6 18L18 6M6 6l12 12"
            />
          </svg>
        </button>
      </div>

      <Droppable droppableId={column.id}>
        {(provided, snapshot) => (
          <div
            ref={provided.innerRef}
            {...provided.droppableProps}
            className={`flex-1 overflow-y-auto space-y-2 min-h-[2rem] rounded-lg p-1 transition-colors ${
              snapshot.isDraggingOver ? "bg-blue-50" : ""
            }`}
          >
            {cards.map((card, index) => (
              <KanbanCard
                key={card.id}
                card={card}
                index={index}
                onClick={() => onCardClick(card)}
              />
            ))}
            {provided.placeholder}
          </div>
        )}
      </Droppable>

      <div className="mt-2">
        {error && (
          <p role="alert" className="mb-2 text-sm text-red-700">
            {error}
          </p>
        )}
        {showAdd ? (
          <form onSubmit={addCard} className="space-y-2">
            <input
              type="text"
              value={newTitle}
              onChange={(e) => setNewTitle(e.target.value)}
              placeholder="Card title..."
              autoFocus
              className="w-full px-3 py-2 text-sm border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500"
            />
            <div className="flex gap-2">
              <button
                type="submit"
                disabled={adding || !newTitle.trim()}
                className="bg-blue-600 text-white text-sm px-3 py-1.5 rounded-lg font-medium hover:bg-blue-700 disabled:opacity-50 transition-colors"
              >
                {adding ? "..." : "Add"}
              </button>
              <button
                type="button"
                onClick={() => {
                  setShowAdd(false);
                  setNewTitle("");
                }}
                className="text-sm text-gray-600 hover:text-gray-800"
              >
                Cancel
              </button>
            </div>
          </form>
        ) : (
          <button
            onClick={() => setShowAdd(true)}
            className="w-full text-left text-sm text-gray-600 hover:text-gray-800 hover:bg-gray-200 rounded-lg px-3 py-2 transition-colors"
          >
            + Add a card
          </button>
        )}
      </div>
    </div>
  );
}
