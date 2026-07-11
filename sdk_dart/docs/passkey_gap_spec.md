# Passkey Gap Spec

## Shipped in `sdk_dart`

- `sdk_dart/lib/src/passkey.dart` defines the pure-Dart
  `PasskeyAuthenticator` seam.
- `sdk_dart/lib/src/client.dart` exposes `AuthClient.signInWithPasskey`, which
  composes `beginWebAuthnLogin`, `PasskeyAuthenticator.authenticate`, and
  `finishWebAuthnLogin`.
- `sdk_dart/lib/allyourbase.dart` exports the seam for downstream packages.

## Deferred Native Implementation

The real native `PasskeyAuthenticator` implementation belongs in a downstream
Flutter package. `sdk_dart/pubspec.yaml` must remain free of Flutter,
platform-channel, and native passkey dependencies so this module stays pure
Dart.

## Validation Boundary

The SDK validates request orchestration and JSON serialization with deterministic
tests. On-device FaceID, biometric, authenticator UX, and platform ceremony
behavior are outside autonomous validation scope for `sdk_dart`.
