# Contributing

Thanks for contributing to Allyourbase.

## Quickstart

1. Follow the public setup flow in `README.md` and `docs-site/guide/getting-started.md`.
2. Start the local server with `ayb start`.
3. Use reproducible CLI validation commands before opening a pull request.

## Validation Requirements

Run automated checks and include exact commands you ran in your PR test plan. Example baseline:

```bash
make test
```

Use additional commands from `docs/howto/testing.md` when your change touches broader surfaces.

Manual-only verification is not sufficient for contributions.

## Issue Routing

- Use the **bug report** issue form for defects.
- Use the **feature request** issue form for product ideas and enhancements.
- Route vulnerability reports to GHSA:
  https://github.com/griddlehq/allyourbase/security/advisories/new

## Pull Request Expectations

Every PR should include:

- A clear summary of what changed and why.
- A concrete test plan with reproducible command output.
- A linked issue or context reference when applicable.
