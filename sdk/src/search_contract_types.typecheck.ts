import type { SearchHit } from "./index";

const highlightedHit: SearchHit<{ id: string; title: string }> = {
  id: "rec_1",
  title: "Postgres guide",
  _highlight: "<b>Postgres</b> guide",
  _highlightResult: {
    title: {
      value: "<b>Postgres</b> guide",
      matchLevel: "full",
    },
  },
};

const titleHighlight: { value: string; matchLevel: string } | undefined = highlightedHit._highlightResult?.title;

void titleHighlight;
