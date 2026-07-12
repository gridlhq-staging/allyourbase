# Swift SDK

Use the Swift `Allyourbase` SDK for iOS/macOS clients with auth, records, realtime (SSE + WebSocket), and storage.

## Install

`sdk_swift/Package.swift` defines package name `Allyourbase` and library product `Allyourbase`.

Preview — install from source. Registry publishing is tracked for GA.

Add `Allyourbase` to your Swift package dependencies from a local checkout of this repository (`Package.swift` is in `sdk_swift/`, not repo root).

Example (local path):

```swift
.package(path: "../sdk_swift")
```

Then depend on product `Allyourbase`.

Full guide: [docs-site/guide/swift-sdk.md](../docs-site/guide/swift-sdk.md).

## Passkey Login

Create a client first:

```swift
import Allyourbase

let ayb = AYBClient("http://localhost:8090")
```

The Swift SDK supports first-factor WebAuthn login on `ayb.auth`:

```swift
let session = try await ayb.auth.signInWithPasskey(email: "dev@allyourbase.io")
```

For custom WebAuthn ceremony handling, use the wire methods directly:

```swift
let begin = try await ayb.auth.beginWebAuthnLogin(email: "dev@allyourbase.io")

let session = try await ayb.auth.finishWebAuthnLogin(
    challengeId: begin.challengeId,
    assertionResponse: assertionResponse
)
```

If your app needs to control `AuthenticationServices` presentation, inject the
system authenticator seam instead of calling the default helper:

```swift
let authenticator = SystemPasskeyAuthenticator(
    presentationContextProvider: presentationContextProvider
)
let session = try await ayb.auth.signInWithPasskey(
    email: "dev@allyourbase.io",
    authenticator: authenticator
)
```

The SDK unit-tests the WebAuthn request/response serialization and HTTP flow.
The native biometric sheet itself is outside autonomous unit validation and
requires device or simulator runtime validation by the app.

OAuth sign-in starts by asking the SDK for the provider URL; the app presents it
with its own browser or `ASWebAuthenticationSession`:

```swift
let url = try ayb.auth.oauthStartURL(
    provider: "github",
    state: state,
    scopes: ["user:email"],
    redirectTo: "myapp://auth/callback"
)
```

For WebAuthn MFA enrollment, use the authenticated wire calls directly or inject
an attestation authenticator for the native ceremony:

```swift
let begin = try await ayb.auth.enrollWebAuthn()
let confirmed = try await ayb.auth.confirmWebAuthnEnrollment(
    displayName: "Primary security key",
    attestationResponse: attestationResponse
)

let enrolled = try await ayb.auth.enrollPasskey(
    displayName: "Primary security key",
    authenticator: authenticator
)
```

When a login returns an MFA-pending token, pass that short-lived token explicitly.
The SDK sends it only on the MFA challenge and verify requests and does not store
it as the session token:

```swift
let challenge = try await ayb.auth.webauthnChallenge(mfaToken: mfaToken)
let session = try await ayb.auth.webauthnVerify(
    mfaToken: mfaToken,
    challengeId: challenge.challengeId,
    assertionResponse: assertionResponse
)

let sessionFromPasskey = try await ayb.auth.verifyPasskey(
    mfaToken: mfaToken,
    authenticator: authenticator
)
```

## Search Synonyms

Typed synonym management lives on `ayb.records` and requires admin auth:

```swift
ayb.setApiKey("your-admin-token")

let updated = try await ayb.records.setSynonyms(
    "posts",
    groups: [
        SearchSynonymGroup(terms: ["ai", "artificial intelligence"]),
        SearchSynonymGroup(terms: ["science fiction", "scifi"]),
    ]
)

let current = try await ayb.records.getSynonyms("posts")
```
