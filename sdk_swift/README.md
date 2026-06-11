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
