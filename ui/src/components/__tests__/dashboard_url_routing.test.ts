import { describe, expect, it } from "vitest";
import {
  SCREEN_REGISTRY,
  filterScreenRegistry,
  findAdminScreen,
  type ScreenRegistry,
} from "../../screens/registry";
import {
  formatDashboardRoute,
  parseDashboardRoute,
  readDashboardAdminBase,
} from "../dashboard_url_routing";

const tables = [
  { schema: "public", name: "users" },
  { schema: "tenant/east", name: "order history" },
];
const location = { search: "?perfRange=24h", hash: "#slow-queries" };

function parse(pathname: string, adminBase = "/admin/", registry = SCREEN_REGISTRY) {
  return parseDashboardRoute(
    { pathname, ...location },
    { adminBase, tables, screenRegistry: registry },
  );
}

describe("parseDashboardRoute", () => {
  it.each([
    ["/admin/", "/admin/screens/sql-editor", "/admin/tables/tenant%2Feast/order%20history"],
    ["/dashboard/", "/dashboard/screens/sql-editor", "/dashboard/tables/tenant%2Feast/order%20history"],
    ["/", "/screens/sql-editor", "/tables/tenant%2Feast/order%20history"],
  ])("maps the closed grammar under base %s", (adminBase, screenPath, tablePath) => {
    expect(parse(adminBase, adminBase)).toEqual({
      route: { kind: "base" },
      location: { pathname: adminBase, ...location },
      historyAction: "none",
    });
    expect(parse(screenPath, adminBase)).toEqual({
      route: { kind: "screen", screen: findAdminScreen("sql-editor") },
      location: { pathname: screenPath, ...location },
      historyAction: "none",
    });
    expect(parse(tablePath, adminBase)).toEqual({
      route: { kind: "table", schema: "tenant/east", table: "order history" },
      location: { pathname: tablePath, ...location },
      historyAction: "none",
    });
  });

  it("returns every closed failure without changing the entered location", () => {
    const capabilityRegistry: ScreenRegistry = {
      sections: SCREEN_REGISTRY.sections.map((section) => ({
        ...section,
        screens: section.screens.map((screen) => (
          screen.id === "sql-editor" ? { ...screen, requires: "storage" } : screen
        )),
      })),
    };
    const hiddenRegistry = filterScreenRegistry(capabilityRegistry, () => false);

    expect(parse("/admin/screens/%E0%A4%A")).toEqual({
      route: { kind: "invalid", label: "Invalid console link" },
      location: { pathname: "/admin/screens/%E0%A4%A", ...location },
      historyAction: "none",
    });
    expect(parse("/admin/screens/not-registered")).toEqual({
      route: { kind: "screen-not-found", label: "Screen not found" },
      location: { pathname: "/admin/screens/not-registered", ...location },
      historyAction: "none",
    });
    expect(parse("/admin/tables/public/missing")).toEqual({
      route: { kind: "table-not-found", label: "Table not found" },
      location: { pathname: "/admin/tables/public/missing", ...location },
      historyAction: "none",
    });
    expect(parse("/admin/tables/public")).toEqual({
      route: { kind: "page-not-found", label: "Page not found" },
      location: { pathname: "/admin/tables/public", ...location },
      historyAction: "none",
    });
    expect(parse("/admin/screens/sql-editor", "/admin/", hiddenRegistry)).toEqual({
      route: { kind: "screen-unavailable", label: "Screen unavailable" },
      location: { pathname: "/admin/screens/sql-editor", ...location },
      historyAction: "none",
    });
    expect(parse("/admin/screens/sql-editor/extra")).toEqual({
      route: { kind: "page-not-found", label: "Page not found" },
      location: { pathname: "/admin/screens/sql-editor/extra", ...location },
      historyAction: "none",
    });
  });

  it.each([
    "/administrator/screens/sql-editor",
    "/administer/screens/sql-editor",
    "/screens/sql-editor",
  ])("rejects pathnames outside the configured base boundary: %s", (pathname) => {
    expect(parse(pathname)).toEqual({
      route: { kind: "page-not-found", label: "Page not found" },
      location: { pathname, ...location },
      historyAction: "none",
    });
  });

  it("canonicalizes only successfully resolved routes with replace semantics", () => {
    expect(parse("/admin/screens/sql%2Deditor/")).toEqual({
      route: { kind: "screen", screen: findAdminScreen("sql-editor") },
      location: { pathname: "/admin/screens/sql-editor", ...location },
      historyAction: "replace",
    });
    expect(parse("/admin/tables/tenant%2feast/order%20history/")).toEqual({
      route: { kind: "table", schema: "tenant/east", table: "order history" },
      location: { pathname: "/admin/tables/tenant%2Feast/order%20history", ...location },
      historyAction: "replace",
    });
  });
});

describe("readDashboardAdminBase", () => {
  it.each([
    ["/console/", "/console/"],
    ["/", "/"],
  ])("reads the emitted admin base %s", (content, expected) => {
    document.head.innerHTML = `<meta name="ayb-admin-base" content="${content}">`;
    expect(readDashboardAdminBase(document)).toBe(expected);
  });

  it("falls back to the default admin base when metadata is absent or invalid", () => {
    document.head.innerHTML = "";
    expect(readDashboardAdminBase(document)).toBe("/admin/");

    document.head.innerHTML = '<meta name="ayb-admin-base" content="console/">';
    expect(readDashboardAdminBase(document)).toBe("/admin/");
  });
});

describe("formatDashboardRoute", () => {
  it.each(["/admin/", "/dashboard/", "/"])("formats exact paths under base %s", (adminBase) => {
    expect(formatDashboardRoute({ kind: "base" }, { adminBase, ...location })).toEqual({
      pathname: adminBase,
      ...location,
    });
    expect(formatDashboardRoute(
      { kind: "screen", screenId: "sql-editor" },
      { adminBase, ...location },
    )).toEqual({
      pathname: `${adminBase}screens/sql-editor`,
      ...location,
    });
    expect(formatDashboardRoute(
      { kind: "table", schema: "tenant/east", table: "order history" },
      { adminBase, ...location },
    )).toEqual({
      pathname: `${adminBase}tables/tenant%2Feast/order%20history`,
      ...location,
    });
  });
});
