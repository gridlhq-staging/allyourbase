import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  GRAPHQL_HISTORY_KEY,
  SCHEMA_INTROSPECTION_QUERY,
  insertGraphqlHistoryEntry,
  loadGraphqlHistory,
  parseGraphqlIntrospectionSchema,
  saveGraphqlHistory,
  type GraphqlHistoryEntry,
} from "../graphql-explorer-helpers";

const introspectionFixture = {
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
            {
              name: "posts",
              description: "List posts.",
              args: [],
              type: {
                kind: "NON_NULL",
                name: null,
                ofType: {
                  kind: "LIST",
                  name: null,
                  ofType: {
                    kind: "NON_NULL",
                    name: null,
                    ofType: { kind: "OBJECT", name: "Post", ofType: null },
                  },
                },
              },
              isDeprecated: false,
              deprecationReason: null,
            },
          ],
        },
        {
          kind: "OBJECT",
          name: "Post",
          description: "A published post.",
          fields: [
            {
              name: "id",
              description: null,
              args: [],
              type: {
                kind: "NON_NULL",
                name: null,
                ofType: { kind: "SCALAR", name: "ID", ofType: null },
              },
              isDeprecated: false,
              deprecationReason: null,
            },
            {
              name: "tags",
              description: "Search labels.",
              args: [
                {
                  name: "matching",
                  description: null,
                  defaultValue: null,
                  type: {
                    kind: "LIST",
                    name: null,
                    ofType: { kind: "SCALAR", name: "String", ofType: null },
                  },
                },
              ],
              type: {
                kind: "LIST",
                name: null,
                ofType: { kind: "SCALAR", name: "String", ofType: null },
              },
              isDeprecated: true,
              deprecationReason: "Use labels instead.",
            },
          ],
        },
      ],
    },
  },
};

function makeEntry(index: number): GraphqlHistoryEntry {
  return {
    query: `query Query${index} { node${index} }`,
    variablesText: `{"limit": ${index}}`,
    status: 200 + index,
    durationMs: index,
    timestamp: `2026-07-19T00:${String(index).padStart(2, "0")}:00.000Z`,
  };
}

describe("graphql-explorer-helpers", () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it("persists known GraphQL history values under the GraphQL-specific key", () => {
    localStorage.setItem("ayb_api_explorer_history", "keep-rest-history");
    const entry = makeEntry(1);
    const persistedEntry = { ...entry, variablesText: "" };

    saveGraphqlHistory([entry]);

    expect(localStorage.getItem("ayb_api_explorer_history")).toBe("keep-rest-history");
    expect(JSON.parse(localStorage.getItem(GRAPHQL_HISTORY_KEY) ?? "[]")).toEqual([
      persistedEntry,
    ]);
    expect(loadGraphqlHistory()).toEqual([persistedEntry]);
  });

  it("returns an empty list for malformed stored JSON", () => {
    localStorage.setItem(GRAPHQL_HISTORY_KEY, "{not json");

    expect(loadGraphqlHistory()).toEqual([]);
  });

  it("does not throw when storage rejects a history save", () => {
    const originalLocalStorage = localStorage;
    const setItem = vi.fn(() => {
      throw new Error("quota exceeded");
    });
    vi.stubGlobal("localStorage", { setItem } as unknown as Storage);

    try {
      expect(() => saveGraphqlHistory([makeEntry(1)])).not.toThrow();
      expect(setItem).toHaveBeenCalledWith(
        GRAPHQL_HISTORY_KEY,
        JSON.stringify([{ ...makeEntry(1), variablesText: "" }]),
      );
    } finally {
      vi.stubGlobal("localStorage", originalLocalStorage);
    }
  });

  it("keeps exactly the newest 20 entries when inserting 21 distinguishable entries", () => {
    let history: GraphqlHistoryEntry[] = [];
    for (let index = 1; index <= 21; index += 1) {
      history = insertGraphqlHistoryEntry(history, makeEntry(index));
    }

    saveGraphqlHistory(history);
    const loaded = loadGraphqlHistory();

    expect(loaded).toHaveLength(20);
    expect(loaded.map((entry) => entry.query)).toEqual(
      Array.from({ length: 20 }, (_, index) => makeEntry(21 - index).query),
    );
    expect(loaded[0]?.variablesText).toBe("");
    expect(loaded).not.toContainEqual({ ...makeEntry(1), variablesText: "" });
  });

  it("scrubs previously persisted variables from existing GraphQL history entries", () => {
    const persistedEntry = makeEntry(3);
    localStorage.setItem(GRAPHQL_HISTORY_KEY, JSON.stringify([persistedEntry]));

    const loaded = loadGraphqlHistory();

    expect(loaded).toEqual([{ ...persistedEntry, variablesText: "" }]);
    expect(JSON.parse(localStorage.getItem(GRAPHQL_HISTORY_KEY) ?? "[]")).toEqual([
      { ...persistedEntry, variablesText: "" },
    ]);
  });

  it("exports the schema query fields required by the parser", () => {
    expect(SCHEMA_INTROSPECTION_QUERY).toContain("query SchemaIntrospection");
    expect(SCHEMA_INTROSPECTION_QUERY).toContain("fields(includeDeprecated: true)");
    expect(SCHEMA_INTROSPECTION_QUERY).toContain("deprecationReason");
    expect(SCHEMA_INTROSPECTION_QUERY).toContain("defaultValue");
  });

  it("parses ordered introspection types, fields, arguments, and nested type references", () => {
    expect(parseGraphqlIntrospectionSchema(introspectionFixture)).toEqual([
      {
        kind: "OBJECT",
        name: "Query",
        description: "Root operations.",
        fields: [
          {
            name: "post",
            description: "Find one post.",
            type: "Post",
            arguments: [
              {
                name: "id",
                description: "Post identifier.",
                type: "ID!",
                defaultValue: null,
              },
            ],
            isDeprecated: false,
            deprecationReason: null,
          },
          {
            name: "posts",
            description: "List posts.",
            type: "[Post!]!",
            arguments: [],
            isDeprecated: false,
            deprecationReason: null,
          },
        ],
      },
      {
        kind: "OBJECT",
        name: "Post",
        description: "A published post.",
        fields: [
          {
            name: "id",
            description: null,
            type: "ID!",
            arguments: [],
            isDeprecated: false,
            deprecationReason: null,
          },
          {
            name: "tags",
            description: "Search labels.",
            type: "[String]",
            arguments: [
              {
                name: "matching",
                description: null,
                type: "[String]",
                defaultValue: null,
              },
            ],
            isDeprecated: true,
            deprecationReason: "Use labels instead.",
          },
        ],
      },
    ]);
  });

  it.each([null, {}, { data: null }, { data: { __schema: { types: null } } }])(
    "returns an empty schema for malformed or partial introspection input %#",
    (input) => {
      expect(parseGraphqlIntrospectionSchema(input)).toEqual([]);
    },
  );
});
