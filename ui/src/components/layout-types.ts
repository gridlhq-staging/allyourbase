/**
 * @module Type definitions for application layout navigation views and utilities to distinguish admin-only views from data-focused views.
 */
/**
 * String literal inventories used to derive the view unions and the runtime
 * admin-view guard from the same source of truth.
 */
import { ADMIN_VIEWS } from "../screens/registry";

type DataView = "data" | "schema" | "sql" | "synonyms" | "search-settings";

export type View = DataView | (typeof ADMIN_VIEWS)[number];

export type AdminView = (typeof ADMIN_VIEWS)[number];

const ADMIN_VIEW_SET: ReadonlySet<AdminView> = new Set(ADMIN_VIEWS);

export function isAdminView(view: View): view is AdminView {
  return ADMIN_VIEW_SET.has(view as AdminView);
}
