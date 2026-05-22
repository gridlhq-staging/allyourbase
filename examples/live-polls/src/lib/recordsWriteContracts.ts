import { ayb } from "./ayb";
import type { Poll, PollOption, Vote } from "../types";

type CreatePollWithOptionsInput = {
  question: string;
  userId: string;
  optionLabels: string[];
};

type CreatePollWithOptionsResult = {
  poll: Poll;
  options: PollOption[];
};

type CastVoteRecordInput = {
  pollId: string;
  optionId: string;
  userId: string;
  existingVoteId?: string;
};

export async function createPollWithOptions({ question, userId, optionLabels }: CreatePollWithOptionsInput): Promise<CreatePollWithOptionsResult> {
  const poll = await ayb.records.create<Poll>("polls", {
    question: question.trim(),
    user_id: userId,
  });

  const normalizedOptionLabels = optionLabels.map((label) => label.trim()).filter(Boolean);
  const options = await Promise.all(
    normalizedOptionLabels.map((label, position) =>
      ayb.records.create<PollOption>("poll_options", {
        poll_id: poll.id,
        label,
        position,
      }),
    ),
  );

  return { poll, options };
}

export async function castVoteRecord({ pollId, optionId, userId, existingVoteId }: CastVoteRecordInput): Promise<Vote> {
  if (existingVoteId) {
    return ayb.records.update<Vote>("votes", existingVoteId, { option_id: optionId });
  }

  return ayb.records.create<Vote>("votes", {
    poll_id: pollId,
    option_id: optionId,
    user_id: userId,
  });
}
