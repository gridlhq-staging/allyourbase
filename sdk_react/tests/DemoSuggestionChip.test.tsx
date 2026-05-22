import React from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { DemoSuggestionChip } from "../src/DemoSuggestionChip";

describe("DemoSuggestionChip", () => {
  it("calls onSelect with suggestion payload", () => {
    const onSelect = vi.fn();
    render(
      <DemoSuggestionChip
        suggestion={{ label: "Alice", email: "alice@demo.test", password: "password123" }}
        onSelect={onSelect}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Alice" }));
    expect(onSelect).toHaveBeenCalledWith({
      label: "Alice",
      email: "alice@demo.test",
      password: "password123",
    });
  });
});
