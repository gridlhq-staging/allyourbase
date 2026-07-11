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

Full guide: [docs-site/guide/kotlin-sdk.md](../docs-site/guide/kotlin-sdk.md).
