/**
 * @module Search settings and synonym management client.
 */
import { encodePathSegment } from "./helpers";
import type {
  SearchSettings,
  SearchSynonymGroup,
  SearchSynonymsRequest,
  SearchSynonymsResponse,
} from "./types";

interface SearchSettingsClientRuntime {
  request<T>(path: string, init?: RequestInit & { skipAuth?: boolean }): Promise<T>;
}

/**
 * Public SDK sub-client for collection search settings and synonym groups.
 * All methods are admin-authenticated and share the same request runtime.
 */
export class SearchSettingsClient {
  constructor(private client: SearchSettingsClientRuntime) {}

  async getSearchSettings(collection: string): Promise<SearchSettings> {
    return this.client.request<SearchSettings>(searchSettingsPath(collection));
  }

  async setSearchSettings(
    collection: string,
    settings: SearchSettings,
  ): Promise<SearchSettings> {
    return this.client.request<SearchSettings>(searchSettingsPath(collection), {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(settings),
    });
  }

  async getSynonyms(collection: string): Promise<SearchSynonymsResponse> {
    return this.client.request<SearchSynonymsResponse>(searchSynonymsPath(collection));
  }

  async setSynonyms(
    collection: string,
    request: SearchSynonymsRequest | SearchSynonymGroup[],
  ): Promise<SearchSynonymsResponse> {
    return this.client.request<SearchSynonymsResponse>(searchSynonymsPath(collection), {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(normalizeSynonymsRequest(request)),
    });
  }
}

function normalizeSynonymsRequest(
  request: SearchSynonymsRequest | SearchSynonymGroup[],
): SearchSynonymsRequest {
  return Array.isArray(request) ? { groups: request } : request;
}

function searchSettingsPath(collection: string): string {
  return `/api/collections/${encodePathSegment(collection)}/search-settings/`;
}

function searchSynonymsPath(collection: string): string {
  return `/api/collections/${encodePathSegment(collection)}/synonyms/`;
}
