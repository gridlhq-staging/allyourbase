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
