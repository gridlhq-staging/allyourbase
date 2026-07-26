export function formatBytes(bytes: number): string {
  if (bytes === 0) return "0 B";
  const k = 1024;
  const sizes = ["B", "KB", "MB", "GB"];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return `${(bytes / Math.pow(k, i)).toFixed(1)} ${sizes[i]}`;
}

export function formatDate(iso: string | null | undefined): string {
  if (!iso) return "-";
  const parsed = new Date(iso);
  return Number.isNaN(parsed.getTime()) ? "-" : parsed.toLocaleString();
}

export function formatCSVCell(value: unknown): string {
  if (value === null || value === undefined) return "";
  let text = typeof value === "object" ? JSON.stringify(value) : String(value);
  const isSpreadsheetFormula =
    typeof value === "string" && /^[=+\-@\t\r]/.test(text);
  if (isSpreadsheetFormula) {
    text = `'${text}`;
  }
  if (
    !isSpreadsheetFormula &&
    !text.includes(",") &&
    !text.includes('"') &&
    !text.includes("\n") &&
    !text.includes("\r")
  ) {
    return text;
  }
  return `"${text.replace(/"/g, '""')}"`;
}

export function formatCSV(rows: readonly (readonly unknown[])[]): string {
  return rows
    .map((row) => row.map((value) => formatCSVCell(value)).join(","))
    .join("\n");
}
