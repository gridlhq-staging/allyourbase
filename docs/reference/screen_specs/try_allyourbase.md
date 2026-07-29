# Try Allyourbase

## Task

Launch a temporary private Allyourbase instance in one click and open its admin dashboard without installing anything.

## Layout

1. The demo landing header explains that these are public Allyourbase examples.
2. A prominent `Try Allyourbase` card explains that the instance is private and expires after 30 minutes.
3. The card contains the anti-abuse challenge and a `Launch my private instance` button.
4. After submission, a status panel announces provisioning progress.
5. When ready, the panel shows `Open Allyourbase`, the temporary admin password, and the expiration time.

## State contract

### Loading

- The launch button is disabled and the status announces `Starting your private Allyourbase instance`.
- The page polls the opaque launch status URL; it never exposes the Daytona API key or AYB JWT secret.

### Error

- A failed launch shows an alert with the server-provided safe error and a `Try again` button.
- Capacity and cooldown errors clearly ask the user to return later.

### Ready

- `Open Allyourbase` opens the signed private preview at `/admin`.
- The randomly generated admin password is visible and can be copied.
- The expiration time states when the sandbox will be destroyed.

## Navigation

- Route: `/`
- Entry: `demo.allyourbase.io`
- Back: browser back follows normal history.
- Open Allyourbase: opens the temporary private admin dashboard in a new tab.

## Acceptance criteria

- Given launch capacity and a completed challenge, when the user presses `Launch my private instance`, then a private one-vCPU Daytona sandbox is created with a 30-minute destruction TTL.
- Given a sandbox is starting, when health is not yet ready, then the page keeps announcing progress and does not expose a dashboard link.
- Given AYB reports both server and database healthy, then the page exposes the signed `/admin` link, temporary password, and expiration time.
- Given the same internet client recently launched an instance, then another launch is rejected before Daytona resources are created.
- Given provisioning fails after sandbox creation, then the sandbox is deleted and no launch is reported as ready.

## Edge cases

- A failed or missing anti-abuse challenge cannot create a sandbox.
- A full global sandbox allowance returns a capacity message without creating a sandbox.
- Expired or unknown launch tokens return an expired message and never reveal another launch.
- Daytona terminal failure states are cleaned up and shown as launch failures.

## Current implementation gaps

None verified.
