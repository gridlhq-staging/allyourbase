import type { SchemaCache, Table } from "../types";

export function collectionLabel(table: Table): string {
  return table.schema === "public" ? table.name : `${table.schema}.${table.name}`;
}

export function selectedHasUnsafeDuplicateName(selected: Table, schema: SchemaCache): boolean {
  if (selected.schema === "public") {
    return false;
  }
  return Object.values(schema.tables).some(
    (table) =>
      table.name === selected.name &&
      (table.schema !== selected.schema || table.kind !== selected.kind),
  );
}
