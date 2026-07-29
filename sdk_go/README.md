# Go SDK

Use `sdk_go` for idiomatic Go access to AYB auth, records, storage, and edge functions.

## Install

Preview - install from a local checkout. The public repo is reachable at
`github.com/gridlhq-staging/allyourbase`, but the SDK module currently declares
`github.com/allyourbase/ayb/sdk_go`, and no live vanity route exposes Go import
metadata for that module path.

```bash
git clone https://github.com/gridlhq-staging/allyourbase.git
cd my-go-app
go mod edit -replace=github.com/allyourbase/ayb/sdk_go=/absolute/path/to/allyourbase/sdk_go
go get github.com/allyourbase/ayb/sdk_go
```

## PostgreSQL RPC

`Client.RPC(ctx, name, args)` returns the function result as `json.RawMessage`.
A 204 or empty response returns `nil`.

```go
ctx := context.Background()
client := allyourbase.NewClient("http://localhost:8080")

raw, err := client.RPC(ctx, "leaderboard_total", map[string]any{
	"club_id": "abc123",
})
if err != nil {
	return err
}
if raw != nil {
	fmt.Println(string(raw))
}
```

## Edge functions

`client.Edge.Invoke(ctx, name, req)` accepts an `EdgeInvokeRequest` and returns
an `EdgeInvokeResponse` with `StatusCode int`, `Headers http.Header`, and
`Body []byte`.

```go
response, err := client.Edge.Invoke(ctx, "send-digest", allyourbase.EdgeInvokeRequest{
	Body:    []byte(`{"club_id":"abc123"}`),
	Headers: map[string]string{"Content-Type": "application/json"},
})
if err != nil {
	return err
}
fmt.Println(response.StatusCode, response.Headers, string(response.Body))
```

GraphQL and admin/org typed clients are JS-only scope today. Go applications
can use AYB REST endpoints or custom HTTP calls for those surfaces.

## Auth

```go
client := allyourbase.NewClient("http://localhost:8080")

redirectTo := "http://localhost:3000/auth/callback"
oauthURL := client.Auth.OAuthStartURL(
	"google",
	"opaque-csrf-state",
	[]string{"email", "profile"},
	&redirectTo,
)
fmt.Println(oauthURL)
```

Use the raw WebAuthn MFA helpers around your own browser authenticator ceremony:

```go
ctx := context.Background()

begin, err := client.Auth.EnrollWebAuthn(ctx)
if err != nil {
	return err
}
_ = begin

attestationResponse := json.RawMessage(`{"id":"credential","rawId":"credential","response":{},"type":"public-key"}`)
_, err = client.Auth.ConfirmWebAuthnEnrollment(ctx, "Primary security key", attestationResponse)
if err != nil {
	return err
}

challenge, err := client.Auth.WebAuthnChallenge(ctx, mfaToken)
if err != nil {
	return err
}

assertionResponse := json.RawMessage(`{"id":"credential","rawId":"credential","response":{},"type":"public-key"}`)
session, err := client.Auth.WebAuthnVerify(ctx, mfaToken, challenge.ChallengeID, assertionResponse)
if err != nil {
	return err
}
fmt.Println(session.User.Email)
```

## Realtime

```go
ctx := context.Background()
client := allyourbase.NewClient("http://localhost:8080", allyourbase.WithAPIKey("ayb_..."))

events, cancel, err := client.Realtime().Subscribe(ctx, "posts", allyourbase.SubscribeOptions{
	Filter: "status=eq.open",
})
if err != nil {
	return err
}
defer cancel()

for event := range events {
	fmt.Printf("%s %s: %#v\n", event.Action, event.Table, event.Record)
}
```

Realtime currently covers one table per websocket subscription and row events
only. Presence, broadcast, multiplexing, SSE, SSR, and pool or performance
tuning are not part of this Go SDK surface yet.

Full guide: [docs-site/guide/go-sdk.md](../docs-site/guide/go-sdk.md).
