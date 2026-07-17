# Marketplace → new-api Model Bridge

Publishes a P2P marketplace script version as a callable new-api model. A call to
`/v1/videos` (OpenAI async video shape) is executed in-process over the existing
E2EE data plane — the relay stays a blind forwarder and the E2EE main path is
unchanged.

## How it fits together

- **Binding** (`model.ScriptModelBinding`): maps a unique `model_name` →
  `(script_id, version)` plus the **publishing operator** (`publisher_user_id`)
  and a default `consume_multiplier` / `param_template`.
- **Channel** (`ChannelTypeMarketplace = 59`): one shared, auto-provisioned
  channel named `aitoken-marketplace`. Publishing adds the model name to its
  `Models` and rebuilds abilities, so the model becomes routable and shows in the
  model square. It has no upstream key/base URL — execution is in-process.
- **Adaptor** (`relay/channel/task/marketplace`): on `POST /v1/videos` it creates
  a marketplace order under the operator, reserves funds, dispatches to a
  provider node, then runs an in-process E2EE client
  (`service/relayclient.RunClientSession`) that handshakes with the provider,
  sends the script config, and receives the encrypted result. The artifact URL is
  written back to the `Task`; `GET /v1/videos/{id}` returns it in OpenAI shape.

## Funding & billing (two layers — read this)

1. **Marketplace order** — funded from the **publishing operator's** marketplace
   *available* balance (`settlement.ReserveFunds`/`Settle`/`Refund`), keyed by
   `binding.publisher_user_id`. The operator must top up that balance on the
   purchase page (`rechargeAvailable`) first, or `DoRequest` fails with a
   reserve-funds error and the task is reported failed. Provider/author/platform
   are paid on receipt reconciliation exactly like a normal marketplace order.

2. **new-api quota** — `/v1/videos` runs under `TokenAuth`, so the **calling
   user** is also pre-charged new-api quota by the model's configured price
   (scaled by the `seconds` ratio, mirroring sora). To avoid double-charging the
   caller, set this model's price to **0** in pricing config (pure pass-through),
   or set a markup deliberately. This is a pricing-config decision, not code.

## Balance-probe exclusion

Scripts designated as a category's balance-probe (`ScriptCategory.balance_script_id`)
only read a site's balance and are never publishable — enforced server-side
(`model.IsBalanceProbeScript`) and hidden in the console UI.

## Operational requirements

- Platform script-signing key must be configured (publishing/execution gate).
- At least one provider node must have the script's capability enabled and be
  online with the target site open/logged in. Provider failures (e.g.
  `ORIGIN_NOT_ALLOWED`) surface as a failed task with the reason.

## Result mapping

The script's E2EE result JSON is scanned for an artifact URL under `url`,
`video_url`, `output_url`, `result_url` (or nested `result`/`output`). The full
result is also embedded in the video `metadata.result`. Base64/inline artifacts
are not yet proxied — support external URLs first (TODO: reuse
`taskcommon.BuildProxyURL` + `VideoProxy` for inline data).

## Building & testing (Go toolchain required)

This bridge was authored in an environment without Go, so it was **not compiled
locally**. Before deploying, run:

```
cd new-api
go build ./...
go test ./service/relayclient/... ./service/dataplane/... ./model/...
```

Key tests:
- `service/relayclient/client_test.go` — full in-process handshake + seal/open
  round trip through a real `relayhub.Hub` and a fake provider, asserting
  byte-level interop with the browser/plugin data-plane protocol.
- `service/dataplane/vector_test.go` — cross-language vector (already present).
- `model/script_model_binding_test.go` — binding CRUD + uniqueness.

## Files

- `model/script_model_binding.go` — binding table + channel provisioning.
- `constant/channel.go` — `ChannelTypeMarketplace`.
- `controller/script_model.go` — publish/unpublish/list endpoints.
- `router/api-router.go` — admin routes under `/api/scripts`.
- `service/relayclient/client.go` — in-process E2EE client.
- `relay/channel/task/marketplace/` — the task adaptor + executor.
- `relay/relay_adaptor.go` — adaptor registration.
- web `features/node-platform/` — `api.ts`, `script-review-console-page.tsx`.

## Known risks / follow-ups

- No Go compile/test run here — needs the checks above.
- Submit-retry double-execution is guarded by an idempotency key derived from the
  public task id.
- Distribute() requires the caller's group to include the marketplace channel's
  group (`default`); adjust the channel group for restricted deployments.
- Inline (base64) artifact proxying is a TODO.
