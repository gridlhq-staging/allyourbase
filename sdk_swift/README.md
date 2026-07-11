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
