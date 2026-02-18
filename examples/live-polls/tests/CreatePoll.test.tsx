import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import CreatePoll from "../src/components/CreatePoll";

// Mock the ayb module.
vi.mock("../src/lib/ayb", () => ({
  ayb: {
    records: {
      create: vi.fn(),
    },
  },
}));

describe("CreatePoll", () => {
  it("renders question input and two option inputs", () => {
    render(<CreatePoll onCreated={vi.fn()} />);

    expect(screen.getByPlaceholderText("Ask a question...")).toBeInTheDocument();
    expect(screen.getByPlaceholderText("Option 1")).toBeInTheDocument();
    expect(screen.getByPlaceholderText("Option 2")).toBeInTheDocument();
  });

  it("renders create button", () => {
    render(<CreatePoll onCreated={vi.fn()} />);
    expect(screen.getByText("Create Poll")).toBeInTheDocument();
  });

  it("can add a third option", () => {
    render(<CreatePoll onCreated={vi.fn()} />);

    fireEvent.click(screen.getByText("+ Add option"));
    expect(screen.getByPlaceholderText("Option 3")).toBeInTheDocument();
  });

  it("cannot remove options below 2", () => {
    render(<CreatePoll onCreated={vi.fn()} />);

    // With only 2 options, no remove buttons should be present.
    expect(screen.queryAllByText("x")).toHaveLength(0);
  });

  it("shows remove buttons when more than 2 options", () => {
    render(<CreatePoll onCreated={vi.fn()} />);

    fireEvent.click(screen.getByText("+ Add option"));
    // Each of the 3 options should have a remove button.
    expect(screen.getAllByText("x")).toHaveLength(3);
  });

  it("shows error when submitting with empty options", async () => {
    render(<CreatePoll onCreated={vi.fn()} />);

    const questionInput = screen.getByPlaceholderText("Ask a question...");
    fireEvent.change(questionInput, { target: { value: "What is best?" } });

    // Options are empty by default, form submit with required fields
    // The validation fires inside handleSubmit for option count.
    const form = screen.getByText("Create Poll").closest("form")!;
    fireEvent.submit(form);

    expect(await screen.findByText("At least 2 options required")).toBeInTheDocument();
  });

  it("hides + Add option at 10 options", () => {
    render(<CreatePoll onCreated={vi.fn()} />);

    // Add 8 more options (start with 2, max is 10).
    for (let i = 0; i < 8; i++) {
      fireEvent.click(screen.getByText("+ Add option"));
    }
    expect(screen.queryByText("+ Add option")).not.toBeInTheDocument();
    expect(screen.getByPlaceholderText("Option 10")).toBeInTheDocument();
  });
});
