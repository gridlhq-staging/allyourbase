import type { Table } from "../types";
import {
  findAdminScreen,
  SCREEN_REGISTRY,
  type AdminScreen,
  type ScreenRegistry,
} from "../screens/registry";

const DEFAULT_ADMIN_BASE = "/admin/";

interface DashboardLocation {
  pathname: string;
  search: string;
  hash: string;
}

export type DashboardRoute =
  | { kind: "base" }
  | { kind: "screen"; screen: AdminScreen }
  | { kind: "table"; schema: string; table: string }
  | { kind: "invalid"; label: "Invalid console link" }
  | { kind: "screen-not-found"; label: "Screen not found" }
  | { kind: "table-not-found"; label: "Table not found" }
  | { kind: "screen-unavailable"; label: "Screen unavailable" }
  | { kind: "page-not-found"; label: "Page not found" };

export type DashboardRouteTarget =
  | { kind: "base" }
  | { kind: "screen"; screenId: string }
  | { kind: "table"; schema: string; table: string };

export interface ParsedDashboardRoute {
  route: DashboardRoute;
  location: DashboardLocation;
  historyAction: "none" | "replace";
}

interface ParseDashboardRouteOptions {
  adminBase: string;
  tables: readonly Pick<Table, "schema" | "name">[];
  screenRegistry: ScreenRegistry;
}

interface FormatDashboardRouteOptions {
  adminBase: string;
  search: string;
  hash: string;
}

export function readDashboardAdminBase(documentOwner: Document = document): string {
  const emittedBase = documentOwner
    .querySelector<HTMLMetaElement>('meta[name="ayb-admin-base"]')
    ?.content;
  return emittedBase && isValidAdminBase(emittedBase)
    ? emittedBase
    : DEFAULT_ADMIN_BASE;
}

function isValidAdminBase(adminBase: string): boolean {
  return adminBase.startsWith("/") && adminBase.endsWith("/") && !adminBase.includes("?") && !adminBase.includes("#");
}

export function parseDashboardRoute(
  location: DashboardLocation,
  options: ParseDashboardRouteOptions,
): ParsedDashboardRoute {
  if (location.pathname === options.adminBase) {
    return result({ kind: "base" }, location);
  }

  if (!location.pathname.startsWith(options.adminBase)) {
    return failure("page-not-found", "Page not found", location);
  }

  const relativePath = location.pathname.slice(options.adminBase.length);
  const rawSegments = relativePath.split("/");
  const hasTrailingSlash = rawSegments[rawSegments.length - 1] === "";
  const routeSegments = hasTrailingSlash ? rawSegments.slice(0, -1) : rawSegments;

  let segments: string[];
  try {
    segments = routeSegments.map((segment) => decodeURIComponent(segment));
  } catch {
    return failure("invalid", "Invalid console link", location);
  }

  if (segments.length === 2 && segments[0] === "screens") {
    return parseScreenRoute(segments[1], location, options, hasTrailingSlash);
  }
  if (segments.length === 3 && segments[0] === "tables") {
    return parseTableRoute(segments[1], segments[2], location, options, hasTrailingSlash);
  }
  return failure("page-not-found", "Page not found", location);
}

function parseScreenRoute(
  screenId: string,
  location: DashboardLocation,
  options: ParseDashboardRouteOptions,
  hasTrailingSlash: boolean,
): ParsedDashboardRoute {
  if (!findAdminScreen(screenId, SCREEN_REGISTRY)) {
    return failure("screen-not-found", "Screen not found", location);
  }
  const screen = findAdminScreen(screenId, options.screenRegistry);
  if (!screen) {
    return failure("screen-unavailable", "Screen unavailable", location);
  }
  const canonicalPathname = formatPath(options.adminBase, "screens", screenId);
  return resolved({ kind: "screen", screen }, location, canonicalPathname, hasTrailingSlash);
}

function parseTableRoute(
  schema: string,
  tableName: string,
  location: DashboardLocation,
  options: ParseDashboardRouteOptions,
  hasTrailingSlash: boolean,
): ParsedDashboardRoute {
  const table = options.tables.find((candidate) => (
    candidate.schema === schema && candidate.name === tableName
  ));
  if (!table) {
    return failure("table-not-found", "Table not found", location);
  }
  const canonicalPathname = formatPath(options.adminBase, "tables", schema, tableName);
  return resolved(
    { kind: "table", schema: table.schema, table: table.name },
    location,
    canonicalPathname,
    hasTrailingSlash,
  );
}

function resolved(
  route: DashboardRoute,
  location: DashboardLocation,
  canonicalPathname: string,
  hasTrailingSlash: boolean,
): ParsedDashboardRoute {
  const canonical = hasTrailingSlash || canonicalPathname !== location.pathname;
  return {
    route,
    location: canonical ? { ...location, pathname: canonicalPathname } : location,
    historyAction: canonical ? "replace" : "none",
  };
}

function result(route: DashboardRoute, location: DashboardLocation): ParsedDashboardRoute {
  return { route, location, historyAction: "none" };
}

function failure(
  kind: "invalid" | "screen-not-found" | "table-not-found" | "screen-unavailable" | "page-not-found",
  label: "Invalid console link" | "Screen not found" | "Table not found" | "Screen unavailable" | "Page not found",
  location: DashboardLocation,
): ParsedDashboardRoute {
  return result({ kind, label } as DashboardRoute, location);
}

export function formatDashboardRoute(
  route: DashboardRouteTarget,
  options: FormatDashboardRouteOptions,
): DashboardLocation {
  let pathname = options.adminBase;
  if (route.kind === "screen") {
    pathname = formatPath(options.adminBase, "screens", route.screenId);
  } else if (route.kind === "table") {
    pathname = formatPath(options.adminBase, "tables", route.schema, route.table);
  }
  return { pathname, search: options.search, hash: options.hash };
}

function formatPath(adminBase: string, namespace: string, ...segments: string[]): string {
  return `${adminBase}${namespace}/${segments.map(encodeURIComponent).join("/")}`;
}
