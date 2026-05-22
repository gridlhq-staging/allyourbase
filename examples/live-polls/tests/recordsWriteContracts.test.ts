import { describe, it, expect, vi, beforeEach } from "vitest";
import { ayb } from "../src/lib/ayb";
import { createPollWithOptions, castVoteRecord } from "../src/lib/recordsWriteContracts";

vi.mock("../src/lib/ayb", () => ({
  ayb: {
    records: {
      create: vi.fn(),
      update: vi.fn(),
    },
  },
}));

const mockCreate = vi.mocked(ayb.records.create);
const mockUpdate = vi.mocked(ayb.records.update);

describe("records write contracts", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("creates poll and ordered options via one owner", async () => {
    mockCreate
      .mockResolvedValueOnce({ id: "poll-1", question: "Q", user_id: "user-1", is_closed: false, created_at: "2026-01-01" })
      .mockResolvedValueOnce({ id: "opt-1", poll_id: "poll-1", label: "A", position: 0 })
      .mockResolvedValueOnce({ id: "opt-2", poll_id: "poll-1", label: "B", position: 1 });

    const result = await createPollWithOptions({ question: " Q ", userId: "user-1", optionLabels: [" A ", "B "] });

    expect(mockCreate).toHaveBeenNthCalledWith(1, "polls", { question: "Q", user_id: "user-1" });
    expect(mockCreate).toHaveBeenNthCalledWith(2, "poll_options", { poll_id: "poll-1", label: "A", position: 0 });
    expect(mockCreate).toHaveBeenNthCalledWith(3, "poll_options", { poll_id: "poll-1", label: "B", position: 1 });
    expect(result.poll.id).toBe("poll-1");
    expect(result.options).toHaveLength(2);
  });

  it("creates first vote and updates existing vote through one owner", async () => {
    mockCreate.mockResolvedValueOnce({ id: "vote-1", poll_id: "poll-1", option_id: "opt-1", user_id: "user-1", created_at: "2026-01-01" });
    mockUpdate.mockResolvedValueOnce({ id: "vote-1", poll_id: "poll-1", option_id: "opt-2", user_id: "user-1", created_at: "2026-01-01" });

    await castVoteRecord({ pollId: "poll-1", optionId: "opt-1", userId: "user-1" });
    await castVoteRecord({ pollId: "poll-1", optionId: "opt-2", userId: "user-1", existingVoteId: "vote-1" });

    expect(mockCreate).toHaveBeenCalledWith("votes", { poll_id: "poll-1", option_id: "opt-1", user_id: "user-1" });
    expect(mockUpdate).toHaveBeenCalledWith("votes", "vote-1", { option_id: "opt-2" });
  });
});
