import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { CollectionSearchSynonymsResponse } from "../../api_admin";
import {
  getCollectionSearchSynonyms,
  updateCollectionSearchSynonyms,
} from "../../api_admin";
import type { SchemaCache, Table } from "../../types";
import { SynonymsEditor } from "../SynonymsEditor";

vi.mock("../../api_admin", () => ({
  getCollectionSearchSynonyms: vi.fn(),
  updateCollectionSearchSynonyms: vi.fn(),
}));

const mockGetSynonyms = vi.mocked(getCollectionSearchSynonyms);
const mockUpdateSynonyms = vi.mocked(updateCollectionSearchSynonyms);

function makeTable(overrides: Partial<Table> = {}): Table {
  return {
    schema: "public",
    name: "books",
    kind: "table",
    columns: [],
    primaryKey: [],
    ...overrides,
  };
}

function makeSchema(tables: Table[] = [makeTable()]): SchemaCache {
  return {
    schemas: [...new Set(tables.map((table) => table.schema))],
    builtAt: "2026-06-04T00:00:00Z",
    tables: Object.fromEntries(
      tables.map((table) => [`${table.schema}.${table.name}`, table]),
    ),
  };
}

function renderEditor(
  response: CollectionSearchSynonymsResponse | Promise<CollectionSearchSynonymsResponse>,
  table = makeTable(),
  schema = makeSchema([table]),
) {
  mockGetSynonyms.mockReturnValueOnce(Promise.resolve(response));
  return render(<SynonymsEditor selected={table} schema={schema} />);
}

async function renderLoadedEditor(
  response: CollectionSearchSynonymsResponse = {
    groups: [{ terms: ["sci-fi", "science fiction"] }],
  },
  table = makeTable(),
  schema = makeSchema([table]),
) {
  renderEditor(response, table, schema);
  await screen.findByDisplayValue("sci-fi");
}

function deferredResponse<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

function setFieldValue(field: HTMLElement, value: string) {
  fireEvent.change(field, { target: { value } });
}

describe("SynonymsEditor", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders seeded groups for the selected public collection and disables save while unchanged", async () => {
    await renderLoadedEditor({
      groups: [
        { terms: ["sci-fi", "science fiction"] },
        { terms: ["ya", "young adult"] },
      ],
    });

    expect(mockGetSynonyms).toHaveBeenCalledWith("books");
    expect(screen.getByRole("heading", { name: "Search synonyms for books" })).toBeInTheDocument();
    expect(screen.getByDisplayValue("sci-fi")).toBeInTheDocument();
    expect(screen.getByDisplayValue("science fiction")).toBeInTheDocument();
    expect(screen.getByDisplayValue("ya")).toBeInTheDocument();
    expect(screen.getByDisplayValue("young adult")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
  });

  it("shows loading state until the first fetch resolves", async () => {
    const deferred = deferredResponse<CollectionSearchSynonymsResponse>();
    renderEditor(deferred.promise);

    expect(screen.getByText("Loading synonyms...")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Add group" })).not.toBeInTheDocument();

    deferred.resolve({ groups: [{ terms: ["mystery", "crime"] }] });

    expect(await screen.findByDisplayValue("mystery")).toBeInTheDocument();
  });

  it("renders the empty state with Add group and enables save only after a valid draft exists", async () => {
    const user = userEvent.setup();
    renderEditor({ groups: [] });

    expect(await screen.findByText("No synonym groups configured")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Add group" }));

    const fields = screen.getAllByLabelText(/Term [12] in group 1/);
    await user.type(fields[0], "space opera");
    await user.type(fields[1], "sci fi");

    expect(screen.getByRole("button", { name: "Save" })).toBeEnabled();
  });

  it("retries fetch failures without changing the selected collection", async () => {
    const user = userEvent.setup();
    mockGetSynonyms
      .mockRejectedValueOnce(new Error("synonym service offline"))
      .mockResolvedValueOnce({ groups: [{ terms: ["romance", "love story"] }] });

    render(<SynonymsEditor selected={makeTable()} schema={makeSchema()} />);

    expect(await screen.findByText("synonym service offline")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Retry" }));

    expect(await screen.findByDisplayValue("romance")).toBeInTheDocument();
    expect(mockGetSynonyms).toHaveBeenCalledTimes(2);
    expect(mockGetSynonyms).toHaveBeenNthCalledWith(2, "books");
  });

  it("disables draft controls while save is in flight", async () => {
    const user = userEvent.setup();
    const deferred = deferredResponse<CollectionSearchSynonymsResponse>();
    await renderLoadedEditor();
    mockUpdateSynonyms.mockReturnValueOnce(deferred.promise);

    await user.type(screen.getByDisplayValue("sci-fi"), " updated");
    await user.click(screen.getByRole("button", { name: "Save" }));

    expect(screen.getByRole("button", { name: "Saving..." })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Add group" })).toBeDisabled();
    expect(screen.getByDisplayValue("sci-fi updated")).toBeDisabled();

    deferred.resolve({ groups: [{ terms: ["sci-fi updated", "science fiction"] }] });
    expect(await screen.findByRole("button", { name: "Save" })).toBeDisabled();
  });

  it("preserves the draft and shows backend save errors", async () => {
    const user = userEvent.setup();
    await renderLoadedEditor();
    mockUpdateSynonyms.mockRejectedValueOnce(new Error("terms rejected by backend"));

    await user.type(screen.getByDisplayValue("science fiction"), " books");
    await user.click(screen.getByRole("button", { name: "Save" }));

    expect(await screen.findByText("terms rejected by backend")).toBeInTheDocument();
    expect(screen.getByDisplayValue("science fiction books")).toBeInTheDocument();
    expect(mockUpdateSynonyms).toHaveBeenCalledTimes(1);
  });

  it("supports add, remove, edit flows and sends the trimmed full-replacement payload", async () => {
    const user = userEvent.setup();
    await renderLoadedEditor({
      groups: [{ terms: ["sci-fi", "science fiction", "speculative fiction"] }],
    });
    mockUpdateSynonyms.mockResolvedValueOnce({
      groups: [
        { terms: ["science fiction", "sf"] },
        { terms: ["space opera", "galactic adventure"] },
      ],
    });

    await user.click(screen.getByRole("button", { name: "Remove term speculative fiction" }));
    setFieldValue(screen.getByLabelText("Term 1 in group 1"), " science fiction ");
    setFieldValue(screen.getByLabelText("Term 2 in group 1"), " sf ");
    await user.click(screen.getByRole("button", { name: "Add group" }));

    const secondGroup = screen.getByRole("group", { name: "Synonym group 2" });
    const secondGroupFields = within(secondGroup).getAllByLabelText(/Term [12] in group 2/);
    setFieldValue(secondGroupFields[0], " space opera ");
    setFieldValue(secondGroupFields[1], " galactic adventure ");
    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => {
      expect(mockUpdateSynonyms).toHaveBeenCalledWith("books", {
        groups: [
          { terms: ["science fiction", "sf"] },
          { terms: ["space opera", "galactic adventure"] },
        ],
      });
    });
    expect(await screen.findByRole("status")).toHaveTextContent("Saved search synonyms.");
  });

  it("blocks saving a group with fewer than two non-empty terms and preserves the draft", async () => {
    const user = userEvent.setup();
    await renderLoadedEditor();

    await user.clear(screen.getByDisplayValue("science fiction"));
    await user.click(screen.getByRole("button", { name: "Save" }));

    expect(screen.getByText("Each synonym group needs at least two terms.")).toBeInTheDocument();
    expect(screen.getByDisplayValue("sci-fi")).toBeInTheDocument();
    expect(mockUpdateSynonyms).not.toHaveBeenCalled();
  });

  it("blocks duplicate normalized terms across groups and preserves the draft", async () => {
    const user = userEvent.setup();
    await renderLoadedEditor({
      groups: [
        { terms: ["sci-fi", "science fiction"] },
        { terms: ["speculative", "fiction"] },
      ],
    });

    setFieldValue(screen.getByLabelText("Term 2 in group 2"), " SCI-FI ");
    await user.click(screen.getByRole("button", { name: "Save" }));

    expect(screen.getByText("Duplicate synonym terms are not allowed.")).toBeInTheDocument();
    expect(screen.getByLabelText("Term 2 in group 2")).toHaveValue(" SCI-FI ");
    expect(mockUpdateSynonyms).not.toHaveBeenCalled();
  });

  it("blocks terms longer than 128 characters and preserves the draft", async () => {
    const user = userEvent.setup();
    const longTerm = "a".repeat(129);
    await renderLoadedEditor();

    setFieldValue(screen.getByDisplayValue("sci-fi"), longTerm);
    await user.click(screen.getByRole("button", { name: "Save" }));

    expect(screen.getByText("Synonym terms must be 128 characters or fewer.")).toBeInTheDocument();
    expect(screen.getByDisplayValue(longTerm)).toBeInTheDocument();
    expect(mockUpdateSynonyms).not.toHaveBeenCalled();
  });

  it("blocks an empty draft after removing all groups", async () => {
    const user = userEvent.setup();
    await renderLoadedEditor();

    await user.click(screen.getByRole("button", { name: "Remove synonym group 1" }));
    await user.click(screen.getByRole("button", { name: "Save" }));

    expect(screen.getByText("Add at least one synonym group before saving.")).toBeInTheDocument();
    expect(screen.getByText("No synonym groups configured")).toBeInTheDocument();
    expect(mockUpdateSynonyms).not.toHaveBeenCalled();
  });

  it("renders non-public duplicate-name selections as read-only without GET or PUT calls", async () => {
    const selected = makeTable({ schema: "private", name: "books" });
    const schema = makeSchema([
      makeTable({ schema: "public", name: "books" }),
      selected,
    ]);

    render(<SynonymsEditor selected={selected} schema={schema} />);

    expect(
      screen.getByText(/unqualified admin route cannot safely target private\.books/i),
    ).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Save" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Add group" })).not.toBeInTheDocument();
    expect(mockGetSynonyms).not.toHaveBeenCalled();
    expect(mockUpdateSynonyms).not.toHaveBeenCalled();
  });

  it("allows public selections even when a non-public table shares the same name", async () => {
    const selected = makeTable({ schema: "public", name: "books" });
    const schema = makeSchema([
      selected,
      makeTable({ schema: "private", name: "books" }),
    ]);

    renderEditor({ groups: [{ terms: ["classic", "literary"] }] }, selected, schema);

    expect(await screen.findByDisplayValue("classic")).toBeInTheDocument();
    expect(mockGetSynonyms).toHaveBeenCalledWith("books");
  });
});
