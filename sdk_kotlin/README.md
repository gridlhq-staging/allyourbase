# Kotlin SDK

Use the Kotlin SDK for AYB auth, records, storage, and realtime (SSE + WebSocket).

## Install

Preview — install from source. Registry publishing is tracked for GA.

```kotlin
// settings.gradle.kts
// after cloning https://github.com/gridlhq-staging/allyourbase.git next to your app
include(":sdk_kotlin")
project(":sdk_kotlin").projectDir = file("../allyourbase/sdk_kotlin")

// app/build.gradle.kts
dependencies {
    implementation(project(":sdk_kotlin"))
}
```

## OAuth

The JVM SDK builds the provider start URL and leaves browser, popup, or custom
redirect handling to your application:

```kotlin
val oauthUrl = ayb.auth.oauthStartUrl(
    provider = "github",
    state = "csrf-state-from-your-app",
    scopes = listOf("user:email", "read:org"),
    redirectTo = "https://app.example.com/post-oauth",
)

// Open oauthUrl in your browser or platform-specific web auth surface, then
// handle the redirect callback in your application.
```

## Passkey login

The JVM SDK owns the WebAuthn wire calls and ceremony composition. Android credential UI stays in your Android app module, so the core SDK does not depend on `androidx.credentials`:

```kotlin
class AndroidPasskeyAuthenticator(
    private val credentialManager: CredentialManager,
    private val context: Context,
) : PasskeyAuthenticator {
    private val json = Json { ignoreUnknownKeys = true }

    override suspend fun createAssertion(options: JsonObject): JsonObject {
        val request = GetCredentialRequest.Builder()
            .addCredentialOption(GetPublicKeyCredentialOption(requestJson = options.toString()))
            .build()
        val credential = credentialManager.getCredential(context, request).credential as PublicKeyCredential
        return json.parseToJsonElement(credential.authenticationResponseJson).jsonObject
    }
}

val auth = ayb.auth.signInWithPasskey(
    "user@example.com",
    AndroidPasskeyAuthenticator(CredentialManager.create(context), context),
)
```

`sdk_kotlin` does not perform on-device biometric validation and does not ship a packaged Android credentials module yet.

## WebAuthn MFA

The core SDK exposes the low-level MFA wire helpers. Your app owns the platform
credential UI and passes the resulting WebAuthn ceremony JSON back to the SDK:

```kotlin
val creationOptions = ayb.auth.enrollWebAuthn()
val attestationResponse: JsonObject = createCredentialWithYourPlatformUi(creationOptions)
ayb.auth.confirmWebAuthnEnrollment(
    displayName = "Primary security key",
    attestationResponse = attestationResponse,
)

val challenge = ayb.auth.webauthnChallenge(mfaToken)
val assertionResponse: JsonObject = getCredentialWithYourPlatformUi(challenge.options)
val session = ayb.auth.webauthnVerify(
    mfaToken = mfaToken,
    challengeId = challenge.challengeId,
    assertionResponse = assertionResponse,
)
```

`webauthnChallenge(...)` and `webauthnVerify(...)` send the pending MFA token as
their bearer token, even if the client already has an existing session.

Full guide: [docs-site/guide/kotlin-sdk.md](../docs-site/guide/kotlin-sdk.md).
