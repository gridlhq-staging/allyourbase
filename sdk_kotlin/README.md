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

Full guide: [docs-site/guide/kotlin-sdk.md](../docs-site/guide/kotlin-sdk.md).
