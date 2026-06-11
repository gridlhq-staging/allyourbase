import { AYBClient } from "@allyourbase/js";
import { createInstantSearchClient } from "@allyourbase/js/instantsearch";

const aybUrl = import.meta.env.VITE_AYB_URL ?? "http://127.0.0.1:8090";

export const ayb = new AYBClient(aybUrl);

export const searchClient = createInstantSearchClient({
  client: ayb,
  objectIDField: "slug",
  highlight: true,
  disjunctiveFacets: ["category"],
});
