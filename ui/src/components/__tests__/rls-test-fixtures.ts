import type { RlsPolicy, SchemaCache } from "../../types";

export function makeRlsSchema(tableNames: string[] = ["posts", "comments"], schemaName = "public"): SchemaCache {
  const tables: SchemaCache["tables"] = {};
  for (const name of tableNames) {
    tables[`${schemaName}.${name}`] = {
      schema: schemaName,
      name,
      kind: "table",
      columns: [
        { name: "id", position: 1, type: "uuid", nullable: false, isPrimaryKey: true, jsonType: "string" },
        { name: "user_id", position: 2, type: "uuid", nullable: false, isPrimaryKey: false, jsonType: "string" },
      ],
      primaryKey: ["id"],
    };
  }
  return { tables, schemas: [schemaName], builtAt: "2026-02-10T12:00:00Z" };
}

export function makePolicy(overrides: Partial<RlsPolicy> = {}): RlsPolicy {
  return {
    tableSchema: "public",
    tableName: "posts",
    policyName: "owner_access",
    command: "ALL",
    permissive: "PERMISSIVE",
    roles: ["authenticated"],
    usingExpr: "(user_id = current_setting('ayb.user_id', true)::uuid)",
    withCheckExpr: "(user_id = current_setting('ayb.user_id', true)::uuid)",
    ...overrides,
  };
}
