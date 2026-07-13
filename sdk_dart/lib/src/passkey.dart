import 'types.dart';

/// Supplies platform passkey assertions for WebAuthn login.
///
/// Implementers run the native device ceremony and return the resulting
/// assertion or attestation response as a JSON-compatible map.
/// The Dart SDK keeps native biometric/passkey UI behind this boundary; client
/// helpers only transport and serialize server WebAuthn options and responses.
abstract class PasskeyAuthenticator {
  const PasskeyAuthenticator();

  Future<JsonMap> authenticate(JsonMap options);

  Future<JsonMap> create(JsonMap options) {
    throw UnsupportedError('Passkey attestation is not implemented.');
  }
}
