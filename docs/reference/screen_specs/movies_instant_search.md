# Movies Instant Search

## Task

Find a movie from a signed-in demo corpus by typing a query, narrowing by primary genre and decade, and selecting a result for notes or chat.

## Layout

1. Header shows `Movies Demo`, signed-in email, and `Sign out`.
2. Hero region has heading `Search the movie corpus`, body copy containing `250 movies`, and a search input labelled `Search movies`.
3. Search controls sit before results: text input, `Primary genre` facet group, `Decade` filter, and `Clear filters`.
4. Results summary is an `aria-live` status such as `Showing 10 of 250 movies`.
5. Results list renders movie result buttons with title, release year, primary genre, overview, and highlighted query matches.
6. Notes panel is reachable after selecting a result and keeps the selected movie title visible.
7. Chat and Provider Keys sections remain after search so signed-in workflows stay reachable.

## State contract

### Loading
- Initial corpus load shows status `Loading movies...` and no result buttons.
- Search-as-you-type updates show status `Searching movies...` without clearing the last successful result list.

### Error
- Corpus or search failure shows `Movie search failed` in an alert and a `Retry search` button.
- Retrying preserves the current query, primary genre, and decade values.

### Signed-in default results
- With an authenticated non-anonymous user and an empty query, the page shows the first 10 seeded movies sorted by relevance fallback, then slug.
- The summary states at least `250 movies` are available.
- Each visible result exposes stable test ids for slug, title, year, primary genre, and overview.

### Search-as-you-type
- Typing `incep` debounces a search request and returns `Inception` in the visible results without pressing a submit button.
- The Inception overview renders the backend highlight payload with accessible label `Highlighted match`.
- Clearing the query returns to signed-in default results.

### Primary genre facets
- The `Primary genre` group lists deterministic facet buttons for seeded primary_genre values, including `Sci-Fi`, `Drama`, `Comedy`, `Action`, and `Documentary`.
- Selecting `Sci-Fi` limits visible result rows to movies whose primary genre is `Sci-Fi` and updates the summary count.
- Facets are single-select; `Clear filters` removes the selected primary genre.

### Decade filtering
- The `Decade` control includes `All decades`, `1980s`, `1990s`, `2000s`, `2010s`, and `2020s`.
- Selecting `2010s` limits visible result rows to release years 2010 through 2019.
- Combining a decade and primary genre applies both filters.

### Empty and no-results
- If the signed-in corpus is empty, show `No seeded movies found` and no facet buttons.
- If filters produce zero matches, show `No movies match your filters` and keep `Clear filters` enabled.

## Navigation

- Route: `/`
- Entry: authenticated movies demo root after anonymous bootstrap or email login.
- Back: browser back follows normal history; filter changes do not push history entries.
- Result select: keeps route stable and reveals notes for the selected movie.
- Sign out: returns to the auth form and clears selected movie, results, and chat history.

## Acceptance criteria

- Given a signed-in user, when the corpus loads, then the page announces at least 250 available movies and renders 10 result buttons. Evidence: `examples/movies/tests/App.search.test.tsx`.
- Given the user types `incep`, when the debounced search settles, then `Inception` is visible and has a highlighted match. Evidence: `examples/movies/tests/App.search.test.tsx`.
- Given the user selects primary genre `Drama`, then every visible result row exposes primary genre `Drama`. Evidence: `examples/movies/tests/App.search.test.tsx`.
- Given the user selects decade `2010s`, then every visible result row has a release year between 2010 and 2019. Evidence: `examples/movies/tests/App.search.test.tsx`.
- Given a search request fails, then an alert with `Movie search failed` and `Retry search` is visible. Evidence: `examples/movies/tests/App.search.test.tsx`.
- Given a mobile viewport 390 pixels wide, then search input, primary genre facets, decade filter, result list, notes, chat, and sign out are reachable by keyboard tab order. Evidence: `examples/movies/tests/App.search.test.tsx`.

## Edge cases

- Anonymous or signed-out users see the existing auth form, not the search surface.
- Empty query plus no filters shows default results rather than an error.
- Whitespace-only query is treated as empty query.
- Offline or server 5xx responses use the error state and keep previous successful results.
- Long movie titles truncate visually but retain full accessible names.

## Implementation notes

- `examples/movies/src/App.tsx` owns the debounced signed-in search state, filter composition, and result selection state.
- `examples/movies/src/lib/ayb.ts` owns the SDK collection-list request contract for search, facets, fuzzy matching, highlighting, filters, and pagination size.
- `examples/movies/src/components/SearchResults.tsx` owns result-row rendering for title, year, primary genre, overview, and highlighted overview payloads.
