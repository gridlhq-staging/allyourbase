import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { executeGraphql } from "../../api";
import type { GraphqlTransportResult } from "../../api_admin";
import { GraphqlExplorer } from "../GraphqlExplorer";
import { SCHEMA_INTROSPECTION_QUERY } from "../graphql-explorer-helpers";

vi.mock("../../api", () => ({
  executeGraphql: vi.fn(),
}));

vi.mock("@uiw/react-codemirror", () => ({
  default: (props: {
    value: string;
    onChange: (value: string) => void;
    placeholder?: string;
    "aria-label"?: string;
  }) => (
    <textarea
      aria-label={props["aria-label"] ?? "GraphQL query"}
      data-testid="graphql-query-editor"
      onChange={(event) => props.onChange(event.target.value)}
      placeholder={props.placeholder}
      value={props.value}
    />
  ),
  EditorView: { contentAttributes: { of: () => [] } },
}));

const mockExecuteGraphql = vi.mocked(executeGraphql);

function makeGraphqlResult(
  overrides: Partial<GraphqlTransportResult> = {},
): GraphqlTransportResult {
  return {
    status: 200,
    statusText: "OK",
    body: { data: { viewer: { id: "user_1" } } },
    durationMs: 12,
    ...overrides,
  };
}

function setTextArea(label: string, value: string) {
  fireEvent.change(screen.getByLabelText(label), { target: { value } });
}

async function sendDefaultQuery() {
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "Send" }));
}

function dispatchShortcut(init: { ctrlKey?: boolean; metaKey?: boolean }) {
  const event = new KeyboardEvent("keydown", {
    key: "Enter",
    bubbles: true,
    cancelable: true,
    ...init,
  });
  const preventDefault = vi.spyOn(event, "preventDefault");
  screen.getByTestId("graphql-explorer").dispatchEvent(event);
  return preventDefault;
}

function deferredGraphqlResult() {
  let resolve: (value: GraphqlTransportResult) => void = () => {};
  const promise = new Promise<GraphqlTransportResult>((promiseResolve) => {
    resolve = promiseResolve;
  });
  return { promise, resolve };
}

describe("GraphqlExplorer", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockExecuteGraphql.mockReset();
    localStorage.clear();
  });

  it("submits the default query with parsed variables and renders the GraphQL response metadata", async () => {
    const body = { data: { posts: [{ id: "post_1", title: "Hello" }] } };
    mockExecuteGraphql.mockResolvedValueOnce(
      makeGraphqlResult({
        body,
        durationMs: 12.6,
      }),
    );
    render(<GraphqlExplorer />);

    const query = screen.getByLabelText("GraphQL query") as HTMLTextAreaElement;
    setTextArea("GraphQL variables", '{"limit": 2}');
    await sendDefaultQuery();

    await waitFor(() => {
      expect(mockExecuteGraphql).toHaveBeenCalledWith(query.value, { limit: 2 });
    });
    expect(screen.getByText("200 OK")).toBeInTheDocument();
    expect(screen.getByText("13ms")).toBeInTheDocument();
    expect(screen.getByTestId("graphql-response-body").textContent).toBe(
      JSON.stringify(body, null, 2),
    );
  });

  it("shows an inline validation error for malformed variables without executing", async () => {
    render(<GraphqlExplorer />);

    setTextArea("GraphQL variables", '{"limit":');
    await sendDefaultQuery();

    expect(
      screen.getByText("Variables must be valid JSON object text."),
    ).toBeInTheDocument();
    expect(mockExecuteGraphql).not.toHaveBeenCalled();
  });

  it.each([
    { name: "meta", init: { metaKey: true } },
    { name: "ctrl", init: { ctrlKey: true } },
  ])("submits once and prevents default on $name Enter", async ({ init }) => {
    mockExecuteGraphql.mockResolvedValue(makeGraphqlResult());
    render(<GraphqlExplorer />);

    let preventDefault: ReturnType<typeof dispatchShortcut> | undefined;
    await act(async () => {
      preventDefault = dispatchShortcut(init);
    });

    if (!preventDefault) {
      throw new Error("Shortcut event was not dispatched");
    }
    expect(preventDefault).toHaveBeenCalledTimes(1);
    await waitFor(() => {
      expect(mockExecuteGraphql).toHaveBeenCalledTimes(1);
    });
  });

  it("ignores keyboard submissions while a GraphQL request is pending", async () => {
    const firstRequest = deferredGraphqlResult();
    mockExecuteGraphql.mockReturnValueOnce(firstRequest.promise);
    render(<GraphqlExplorer />);

    let firstPreventDefault: ReturnType<typeof dispatchShortcut> | undefined;
    let secondPreventDefault: ReturnType<typeof dispatchShortcut> | undefined;
    await act(async () => {
      firstPreventDefault = dispatchShortcut({ metaKey: true });
      secondPreventDefault = dispatchShortcut({ ctrlKey: true });
    });

    if (!firstPreventDefault || !secondPreventDefault) {
      throw new Error("Shortcut events were not dispatched");
    }
    expect(firstPreventDefault).toHaveBeenCalledTimes(1);
    expect(secondPreventDefault).toHaveBeenCalledTimes(1);
    expect(mockExecuteGraphql).toHaveBeenCalledTimes(1);

    await act(async () => {
      firstRequest.resolve(
        makeGraphqlResult({
          status: 202,
          statusText: "Accepted",
          durationMs: 25,
        }),
      );
      await firstRequest.promise;
    });

    expect(screen.getByText("202 Accepted")).toBeInTheDocument();
    expect(screen.getByText("History (1)")).toBeInTheDocument();
  });

  it("preserves GraphQL data and errors in the response pane", async () => {
    const body = {
      data: { post: null },
      errors: [{ message: "post was not found" }],
    };
    mockExecuteGraphql.mockResolvedValueOnce(
      makeGraphqlResult({
        status: 200,
        statusText: "OK",
        body,
      }),
    );
    render(<GraphqlExplorer />);

    await sendDefaultQuery();

    await waitFor(() => {
      expect(screen.getByTestId("graphql-response-body").textContent).toBe(
        JSON.stringify(body, null, 2),
      );
    });
    expect(screen.queryByText("post was not found", { selector: "[role=alert] *" }))
      .not.toBeInTheDocument();
  });

  it("loads and renders schema types without recording introspection in history", async () => {
    mockExecuteGraphql.mockResolvedValueOnce(
      makeGraphqlResult({
        body: {
          data: {
            __schema: {
              types: [
                {
                  kind: "OBJECT",
                  name: "Query",
                  description: "Root operations.",
                  fields: [
                    {
                      name: "post",
                      description: "Find one post.",
                      args: [
                        {
                          name: "id",
                          description: "Post identifier.",
                          defaultValue: null,
                          type: {
                            kind: "NON_NULL",
                            name: null,
                            ofType: { kind: "SCALAR", name: "ID", ofType: null },
                          },
                        },
                      ],
                      type: { kind: "OBJECT", name: "Post", ofType: null },
                      isDeprecated: false,
                      deprecationReason: null,
                    },
                  ],
                },
              ],
            },
          },
        },
      }),
    );
    render(<GraphqlExplorer />);

    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Load schema" }));

    await waitFor(() => {
      expect(mockExecuteGraphql).toHaveBeenCalledWith(SCHEMA_INTROSPECTION_QUERY);
    });
    await user.click(screen.getByText("Query", { selector: "summary *" }));

    const field = screen.getByTestId("graphql-schema-field-Query-post");
    expect(within(field).getByText("post", { selector: "span" })).toBeInTheDocument();
    expect(within(field).getByText("Post", { selector: "span" })).toBeInTheDocument();
    const argument = screen.getByTestId("graphql-schema-argument-Query-post-id");
    expect(within(argument).getByText("id", { selector: "code" })).toBeInTheDocument();
    expect(within(argument).getByText("ID!", { selector: "code" })).toBeInTheDocument();
    expect(screen.getByText("History (0)")).toBeInTheDocument();
    expect(localStorage.getItem("ayb_graphql_explorer_history")).toBeNull();
  });

  it.each([
    {
      name: "HTTP 403",
      schemaResult: makeGraphqlResult({
        status: 403,
        statusText: "Forbidden",
        body: { errors: [{ message: "introspection requires admin access" }] },
      }),
    },
    {
      name: "status-200 introspection errors envelope",
      schemaResult: makeGraphqlResult({
        body: { errors: [{ message: "introspection is disabled" }] },
      }),
    },
  ])(
    "renders the closed schema message for $name while normal queries remain usable",
    async ({ schemaResult }) => {
      const queryBody = {
        data: { post: null },
        errors: [{ message: "post was not found" }],
      };
      mockExecuteGraphql
        .mockResolvedValueOnce(schemaResult)
        .mockResolvedValueOnce(makeGraphqlResult({ body: queryBody }));
      const consoleError = vi.spyOn(console, "error").mockImplementation(() => {});
      const unhandledRejection = vi.fn();
      window.addEventListener("unhandledrejection", unhandledRejection);

      try {
        render(<GraphqlExplorer />);
        const user = userEvent.setup();
        await user.click(screen.getByRole("button", { name: "Load schema" }));

        expect(
          await screen.findByText(
            "Schema browsing requires admin access or is disabled",
          ),
        ).toBeInTheDocument();
        expect(screen.queryByRole("alert")).not.toBeInTheDocument();

        await user.click(screen.getByRole("button", { name: "Send" }));
        await waitFor(() => {
          expect(screen.getByTestId("graphql-response-body").textContent).toBe(
            JSON.stringify(queryBody, null, 2),
          );
        });
        expect(mockExecuteGraphql).toHaveBeenCalledTimes(2);
        expect(consoleError).not.toHaveBeenCalled();
        expect(unhandledRejection).not.toHaveBeenCalled();
      } finally {
        window.removeEventListener("unhandledrejection", unhandledRejection);
        consoleError.mockRestore();
      }
    },
  );

  it("renders the disabled-server message for a non-JSON 404 without request error noise", async () => {
    const queryBody = { data: { viewer: { id: "user_1" } } };
    mockExecuteGraphql
      .mockResolvedValueOnce(
        makeGraphqlResult({
          status: 404,
          statusText: "Not Found",
          body: { errors: [{ message: "GraphQL response was not valid JSON" }] },
        }),
      )
      .mockResolvedValueOnce(makeGraphqlResult({ body: queryBody }));
    const consoleError = vi.spyOn(console, "error").mockImplementation(() => {});
    const unhandledRejection = vi.fn();
    window.addEventListener("unhandledrejection", unhandledRejection);

    try {
      render(<GraphqlExplorer />);
      const user = userEvent.setup();
      await user.click(screen.getByRole("button", { name: "Load schema" }));

      expect(
        await screen.findByText("GraphQL is not enabled on this server"),
      ).toBeInTheDocument();
      expect(screen.queryByText("404 Not Found")).not.toBeInTheDocument();
      expect(
        screen.queryByText("GraphQL response was not valid JSON"),
      ).not.toBeInTheDocument();
      expect(screen.queryByRole("alert")).not.toBeInTheDocument();

      await user.click(screen.getByRole("button", { name: "Send" }));
      await waitFor(() => {
        expect(screen.getByTestId("graphql-response-body").textContent).toBe(
          JSON.stringify(queryBody, null, 2),
        );
      });
      expect(mockExecuteGraphql).toHaveBeenCalledTimes(2);
      expect(consoleError).not.toHaveBeenCalled();
      expect(unhandledRejection).not.toHaveBeenCalled();
    } finally {
      window.removeEventListener("unhandledrejection", unhandledRejection);
      consoleError.mockRestore();
    }
  });

  it("persists completed requests without stored variables, replays editor text, and clears history", async () => {
    mockExecuteGraphql.mockResolvedValueOnce(
      makeGraphqlResult({
        status: 206,
        statusText: "Partial Content",
        durationMs: 19,
      }),
    );
    const firstRender = render(<GraphqlExplorer />);

    setTextArea("GraphQL query", "query Posts($limit: Int) { posts(limit: $limit) { id } }");
    setTextArea("GraphQL variables", '{"limit": 2}');
    await sendDefaultQuery();

    await waitFor(() => {
      expect(screen.getByText("206 Partial Content")).toBeInTheDocument();
    });
    expect(screen.getByText("History (1)")).toBeInTheDocument();
    expect(JSON.parse(localStorage.getItem("ayb_graphql_explorer_history") ?? "[]")).toEqual([
      expect.objectContaining({
        query: "query Posts($limit: Int) { posts(limit: $limit) { id } }",
        variablesText: "",
        status: 206,
        durationMs: 19,
      }),
    ]);

    const currentSession = screen.getByText("History (1)");
    await userEvent.setup().click(currentSession);
    const currentSessionRow = screen.getByRole("button", {
      name: /query Posts\(\$limit: Int\).*206.*19ms/,
    });
    await userEvent.setup().click(currentSessionRow);
    expect(screen.getByLabelText("GraphQL variables")).toHaveValue('{"limit": 2}');

    firstRender.unmount();
    vi.clearAllMocks();
    render(<GraphqlExplorer />);

    const user = userEvent.setup();
    await user.click(screen.getByText("History (1)"));

    const row = screen.getByRole("button", {
      name: /query Posts\(\$limit: Int\).*206.*19ms/,
    });
    expect(row).toBeInTheDocument();
    await user.click(row);

    expect(screen.getByLabelText("GraphQL query")).toHaveValue(
      "query Posts($limit: Int) { posts(limit: $limit) { id } }",
    );
    expect(screen.getByLabelText("GraphQL variables")).toHaveValue("");
    expect(mockExecuteGraphql).not.toHaveBeenCalled();

    await user.click(screen.getByText("History (1)"));
    await user.click(screen.getByRole("button", { name: "Clear" }));

    expect(screen.getByText("History (0)")).toBeInTheDocument();
    expect(localStorage.getItem("ayb_graphql_explorer_history")).toBe("[]");
  });

  it("surfaces a thrown transport error without recording history or a stale response", async () => {
    mockExecuteGraphql
      .mockResolvedValueOnce(makeGraphqlResult({ status: 206, durationMs: 19 }))
      .mockRejectedValueOnce(new Error("Unauthorized"));
    render(<GraphqlExplorer />);

    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Send" }));
    await waitFor(() => {
      expect(screen.getByTestId("graphql-response-body")).toBeInTheDocument();
    });
    expect(screen.getByText("History (1)")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Send" }));

    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent("Unauthorized");
    // The failed attempt must clear the previous response rather than leave it stale.
    expect(screen.queryByTestId("graphql-response-body")).not.toBeInTheDocument();
    // A thrown request never completed, so it must not enter history.
    expect(screen.getByText("History (1)")).toBeInTheDocument();
    expect(
      JSON.parse(localStorage.getItem("ayb_graphql_explorer_history") ?? "[]"),
    ).toHaveLength(1);
    expect(screen.getByRole("button", { name: "Send" })).toBeEnabled();
  });

  it("renders the schema load error message when introspection transport throws", async () => {
    mockExecuteGraphql.mockRejectedValueOnce(new Error("network down"));
    render(<GraphqlExplorer />);

    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Load schema" }));

    expect(
      await screen.findByText("Unable to load the GraphQL schema"),
    ).toBeInTheDocument();
    // A schema failure is reported in the schema pane only, not the response alert.
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Load schema" })).toBeEnabled();
  });
});
