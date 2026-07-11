import 'types.dart';

/// Supplies platform passkey assertions for WebAuthn login.
///
/// Implementers run the native device ceremony and return the resulting
/// assertion response as a JSON-compatible map.
abstract class PasskeyAuthenticator {
  const PasskeyAuthenticator();

  Future<JsonMap> authenticate(JsonMap options);
}
