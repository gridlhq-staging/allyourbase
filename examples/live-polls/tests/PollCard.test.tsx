import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import PollCard from "../src/components/PollCard";
import type { Poll, PollOption, Vote } from "../src/types";

// Mock the ayb module.
vi.mock("../src/lib/ayb", () => ({
  ayb: {
    token: "test-token",
    records: {
      update: vi.fn(),
    },
  },
}));

const poll: Poll = {
  id: "poll-1",
  user_id: "user-owner",
  question: "What is the best language?",
  is_closed: false,
  created_at: "2026-02-09T00:00:00Z",
};

const options: PollOption[] = [
  { id: "opt-a", poll_id: "poll-1", label: "TypeScript", position: 0 },
  { id: "opt-b", poll_id: "poll-1", label: "Go", position: 1 },
  { id: "opt-c", poll_id: "poll-1", label: "Rust", position: 2 },
];

function makeVotes(counts: Record<string, number>): Vote[] {
  const votes: Vote[] = [];
  let userIdx = 0;
  for (const [optionId, count] of Object.entries(counts)) {
    for (let i = 0; i < count; i++) {
      votes.push({
        id: `vote-${userIdx}`,
        poll_id: "poll-1",
        option_id: optionId,
        user_id: `user-${userIdx}`,
        created_at: "2026-02-09T00:00:00Z",
      });
      userIdx++;
    }
  }
  return votes;
}

describe("PollCard", () => {
  it("renders the poll question", () => {
    render(
      <PollCard poll={poll} options={options} votes={[]} currentUserId={null} onClose={vi.fn()} />,
    );
    expect(screen.getByText("What is the best language?")).toBeInTheDocument();
  });

  it("renders all options", () => {
    render(
      <PollCard poll={poll} options={options} votes={[]} currentUserId={null} onClose={vi.fn()} />,
    );
    expect(screen.getByText("TypeScript")).toBeInTheDocument();
    expect(screen.getByText("Go")).toBeInTheDocument();
    expect(screen.getByText("Rust")).toBeInTheDocument();
  });

  it("shows vote counts and percentages", () => {
    const votes = makeVotes({ "opt-a": 3, "opt-b": 1 });
    render(
      <PollCard poll={poll} options={options} votes={votes} currentUserId={null} onClose={vi.fn()} />,
    );
    expect(screen.getByText("3 votes (75%)")).toBeInTheDocument();
    expect(screen.getByText("1 vote (25%)")).toBeInTheDocument();
    expect(screen.getByText("0 votes (0%)")).toBeInTheDocument();
    expect(screen.getByText("4 total votes")).toBeInTheDocument();
  });

  it("shows zero votes for empty poll", () => {
    render(
      <PollCard poll={poll} options={options} votes={[]} currentUserId={null} onClose={vi.fn()} />,
    );
    expect(screen.getByText("0 total votes")).toBeInTheDocument();
  });

  it("shows checkmark on user's voted option", () => {
    const votes = makeVotes({ "opt-a": 1 });
    // user-0 voted for opt-a (from makeVotes).
    render(
      <PollCard
        poll={poll}
        options={options}
        votes={votes}
        currentUserId="user-0"
        onClose={vi.fn()}
      />,
    );
    // The checkmark character is rendered before the label.
    const tsOption = screen.getByText(/TypeScript/);
    expect(tsOption.textContent).toContain("\u2713");
  });

  it("shows Closed badge when poll is closed", () => {
    const closedPoll = { ...poll, is_closed: true };
    render(
      <PollCard
        poll={closedPoll}
        options={options}
        votes={[]}
        currentUserId={null}
        onClose={vi.fn()}
      />,
    );
    expect(screen.getByText("Closed")).toBeInTheDocument();
  });

  it("shows close button for poll owner", () => {
    render(
      <PollCard
        poll={poll}
        options={options}
        votes={[]}
        currentUserId="user-owner"
        onClose={vi.fn()}
      />,
    );
    expect(screen.getByText("Close poll")).toBeInTheDocument();
  });

  it("hides close button for non-owners", () => {
    render(
      <PollCard
        poll={poll}
        options={options}
        votes={[]}
        currentUserId="user-other"
        onClose={vi.fn()}
      />,
    );
    expect(screen.queryByText("Close poll")).not.toBeInTheDocument();
  });

  it("options are sorted by position", () => {
    const reversed: PollOption[] = [
      { id: "opt-c", poll_id: "poll-1", label: "Rust", position: 2 },
      { id: "opt-a", poll_id: "poll-1", label: "TypeScript", position: 0 },
      { id: "opt-b", poll_id: "poll-1", label: "Go", position: 1 },
    ];
    render(
      <PollCard
        poll={poll}
        options={reversed}
        votes={[]}
        currentUserId={null}
        onClose={vi.fn()}
      />,
    );
    const buttons = screen.getAllByRole("button").filter((b) => b.textContent?.match(/votes/));
    expect(buttons[0].textContent).toContain("TypeScript");
    expect(buttons[1].textContent).toContain("Go");
    expect(buttons[2].textContent).toContain("Rust");
  });
});
