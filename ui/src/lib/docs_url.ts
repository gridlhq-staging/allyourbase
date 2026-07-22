const DOCUMENTATION_ORIGIN = "https://allyourbase.io";

export type GuidePath = `/guide/${string}`;

export function docsUrl(path: GuidePath): string {
  return `${DOCUMENTATION_ORIGIN}${path}`;
}
