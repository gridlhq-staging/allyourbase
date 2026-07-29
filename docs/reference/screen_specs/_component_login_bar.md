<!-- Spec source: chats/icg/may24_1119am_4_screen_specs_scaffold_and_login_bar_component.md
     Component source: sdk_react/src/AybLoginBar.tsx
     Props interface source: sdk_react/src/types.ts -->
# AybLoginBar

## Task

Authenticate a user with the shared `AybLoginBar` widget using password, magic-link, OAuth, anonymous, or anonymous-upgrade flows based on the enabled methods and callbacks.

## Layout

1. Password branch (when `methods.password=true`): `Email` input, `Password` input, primary submit button, and optional mode-toggle prompt/button when `onModeChange` is provided.
2. Magic-link button branch (when `methods.magicLink=true` and `onRequestMagicLink` exists): `Email me a magic link` button.
3. OAuth branch (when `methods.oauth=true`): either provider-specific buttons (`Continue with GitHub`, `Continue with Google`) when `oauthProviders` has entries and `onOAuthProvider` is supplied, or a single fallback `Continue with OAuth` button otherwise.
4. Anonymous branch (when `methods.anonymous=true`): `Continue as Guest` button.
5. Upgrade-anonymous branch (when `methods.canUpgradeAnonymous=true`): `Upgrade Account` button when `onUpgradeAnonymous` exists, otherwise informational text `Upgrade your guest account`.
6. Error message (when `error` is non-empty): paragraph with `role="alert"` showing the error text.
7. Demo suggestion chips list: renders `DemoSuggestionChip` entries and fills email/password into inputs when selected.

## State contract

### Loading
- All rendered buttons are disabled when `loading=true` (submit, mode-toggle, magic-link, OAuth, anonymous, and upgrade).
- Input values remain controlled by props (`email`, `password`) while loading.

### Error
- When `error` is non-null, the widget renders a visible message in an element with `role="alert"`.
- Error content is fully controlled by the embedding screen through the `error` prop.

### Password sign-in or register
- Password submit button is rendered only when `methods.password=true`.
- Submit enablement requires all of: `methods.password=true`, `email.length > 0`, and `password.length > 0`; otherwise submit stays disabled.
- Submit label defaults by mode: `Sign In` for `login`, `Create Account` for `register`, unless `submitLabel` overrides it.
- Mode-toggle prompt and button render only when `onModeChange` is provided; clicking toggles between `login` and `register`.

### Magic-link request
- Magic-link button renders only when both `methods.magicLink=true` and `onRequestMagicLink` are present.
- Magic-link button enablement requires `email.length > 0`; click invokes `onRequestMagicLink(email)`.

### OAuth
- If `methods.oauth=true` and both `oauthProviders` has one or more providers and `onOAuthProvider` exists, render one button per provider using provider-specific labels.
- Otherwise, render exactly one generic fallback button `Continue with OAuth` that invokes `onOAuth()`.

### Anonymous and upgrade
- If `methods.anonymous=true`, render `Continue as Guest` and invoke `onAnonymous()` on click.
- If `methods.canUpgradeAnonymous=true` and `onUpgradeAnonymous` exists, render `Upgrade Account` gated by the same submit gate as password flow (`email` and `password` required).
- If `methods.canUpgradeAnonymous=true` and `onUpgradeAnonymous` is absent, render fallback text instead of an upgrade button.

## Navigation

- Route: Not applicable at widget scope; `AybLoginBar` does not own routing.
- Entry: Embedded by per-demo auth screens (for example each demo-specific `Login` screen).
- Back: Not applicable at widget scope; parent screen owns back behavior.
- `Login`: Future per-demo `Login` specs own route transitions into and out of this widget.

## Acceptance criteria

- Given `methods.password=true`, `loading=false`, `email="a@b.com"`, and `password="pw"`, when the widget renders, then enabled password submit and optional mode-toggle behavior match the passed mode/callback props.
- Given `methods.magicLink=true`, `onRequestMagicLink` supplied, and `email="a@b.com"`, when the user clicks `Email me a magic link`, then `onRequestMagicLink("a@b.com")` is invoked.
- Given `methods.oauth=true` with `oauthProviders=["github","google"]` and `onOAuthProvider` supplied, when the widget renders, then exactly provider-specific OAuth buttons are shown.
- Given `methods.oauth=true` with `oauthProviders` empty/undefined or `onOAuthProvider` not supplied, when the widget renders, then one generic `Continue with OAuth` button is shown. Evidence: `sdk_react/tests/AybLoginBar.test.tsx`.
- Given `error="Authentication failed"`, when the widget renders, then an element with `role="alert"` and that text is visible. Evidence owner: no deterministic assertion exists yet; see `Current implementation gaps`.
- Given any rendered button and `loading=true`, when the widget renders, then that button is disabled. Evidence owner: no deterministic assertion exists yet; see `Current implementation gaps`.

## Edge cases

- No methods enabled: widget renders no actionable controls; embedding screen is responsible for deciding whether this configuration is valid.
- `onModeChange` omitted: mode-toggle prompt/button block is not rendered; parent controls mode externally.
- OAuth callback split: when providers exist but `onOAuthProvider` is absent, generic OAuth fallback is rendered instead of provider buttons.
- Live wrapper variance: demo wrappers may pass different `methods` combinations (`canUpgradeAnonymous` true/false, dynamic anonymous mode), but all behavior still follows this shared widget contract.
- Magic-link-only configuration: current implementation can render magic-link button without rendering an email input, creating a user-input gap captured below.

## Current implementation gaps

- Current: Mode-variable controls (primary submit label and mode-toggle label) have no `data-testid` attributes in `AybLoginBar`; selectors must rely on changing visible text by mode.
- Target: Add narrow stable test IDs only for mode-variable controls (submit button and mode-toggle button), while leaving stable role/name-locatable controls unchanged.
- Evidence: `sdk_react/src/AybLoginBar.tsx:61-72` and Stage 1 verified delta in `/Users/stuart/.matt/projects/allyourbase_dev-7cc0f9f4/may24_1119am_4_screen_specs_scaffold_and_login_bar_component.md-1e411437/checklists/stage_01_checklist.md:12-17`.

- Current: Email input is rendered only inside the password branch, but the magic-link branch can render independently when `methods.magicLink=true` and `onRequestMagicLink` exists.
- Target: Render an email input when password auth or magic-link auth is enabled so magic-link-only configurations still collect an email before request.
- Evidence: `sdk_react/src/AybLoginBar.tsx:46-53,78-85` and Stage 1 verified delta in `/Users/stuart/.matt/projects/allyourbase_dev-7cc0f9f4/may24_1119am_4_screen_specs_scaffold_and_login_bar_component.md-1e411437/checklists/stage_01_checklist.md:12-17`.

- Current: The current test corpus has no deterministic assertion for the shared `AybLoginBar` error-alert rendering or for all rendered buttons being disabled while `loading=true`.
- Target: Add component proof that asserts the `role="alert"` error text and disabled state across rendered password, mode-toggle, magic-link, OAuth, anonymous, and upgrade controls without changing this target spec.
- Evidence: stage 2 proof sweep over `sdk_react/tests/AybLoginBar.test.tsx`, `sdk_react/tests/contract.test.tsx`, `examples/movies/tests/AuthForm.test.tsx`, and `examples/instantsearch_demo/tests/App.test.tsx`.
