# Movies Search Demo

An instant movie search showcase built with Allyourbase. It demonstrates how AYB can replace an Algolia-style hosted search layer with built-in Postgres full-text search, typo-tolerant fuzzy matching, filters, facets, auth, and local AI hooks.

## Quick Start

```bash
ayb demo movies
```

Sign in with one of the demo accounts printed by the command, search the seeded corpus, select a result, save a note, and ask a chat question.

### Manual Setup

```bash
ayb start --config ayb.toml
ayb sql < schema.sql
ayb sql < seed.sql
npm install
npm run dev
```

`ayb.toml` owns the local AI provider settings, `schema.sql` owns the database shape, and `seed.sql` owns the deterministic corpus.

## Demonstrates

- **Instant collection search** - search-as-you-type over the standard records list API
- **Fuzzy matching and highlighting** - typo-tolerant title and overview matches
- **Facets and filters** - `primary_genre` facet buckets and decade filtering compose on the same request path
- **Authentication** - seeded email/password demo accounts
- **Vector-backed notes** - notes are embedded against the selected movie
- **Streaming chat** - local deterministic chat responses for the browser-unmocked harness
- **Bring your own key** - provider-key storage flow backed by AYB vault endpoints

## Schema

See [schema.sql](./schema.sql) for the complete schema and [seed.sql](./seed.sql) for the deterministic corpus.

```text
movies (slug, title, overview, release_year, genres, primary_genre, embedding)
  -> movies_notes (movie_slug, text, embedding)
  -> movies_chat_history (session_id, role, content, partial)
```

`primary_genre` is the scalar facet owner for genre buckets. `genres` keeps the broader tag list for each seeded movie.

## Search Guide

The demo uses AYB's standard collection search surface rather than a search-only endpoint. See [Search](../../docs-site/guide/search.md) for the full API contract.

## Testing

```bash
npm test
npm run lint:browser-tests
npm run test:e2e
```

`npm run test:e2e` runs the browser-unmocked Playwright suite. Its web server command is [e2e/run_demo_with_fake_ollama.sh](./e2e/run_demo_with_fake_ollama.sh), which starts the existing movies demo path with the deterministic fake Ollama provider used by the tests.
