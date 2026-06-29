# HTTP Endpoints Requiring Auth Middleware

As of the gRPC auth enforcement change, all gRPC endpoints (including
HTTP-to-gRPC gateway routes at `/v1/`) now require a valid OIDC Bearer
token when auth methods are configured.

The following HTTP-only endpoints bypass the gRPC interceptor and do NOT
enforce authentication server-side. They need separate HTTP auth
middleware.

## Endpoints

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/api/ui/config` | GET | Returns OIDC configuration for frontend login |
| `/api/ui/user-config` | GET | Returns UI layout, plugin registry, assets host |
| `/api/ui/setup/ws` | WebSocket | Streams day-one setup status events |
| `/api/ui/events/ws` | WebSocket | Streams delivery event updates |
| `/api/ui/github-signing-keys/{username}` | GET | Proxies GitHub SSH public keys |
| `/api/ui/verify-sign` | POST | Verifies cryptographic signatures (**already enforces auth**) |

## Notes

- The frontend fetch interceptor (`packages/gui/src/auth/fetchInterceptor.ts`)
  already injects `Authorization: Bearer {token}` on all same-origin
  requests, so tokens are already being sent to these endpoints.
- The server-side gap is that these HTTP handlers do not validate the token.
- `/api/ui/verify-sign` already extracts and verifies the Bearer token
  independently — no change needed there.
- `/api/ui/config` may need to remain unauthenticated so the frontend can
  fetch OIDC configuration before the user has logged in (chicken-and-egg).
  Evaluate whether this endpoint exposes sensitive data.
- All routes are registered in `fleetshift-server/internal/cli/serve.go`
  on the `topMux` (lines 509-517).
- The existing `MaxBody` middleware in
  `fleetshift-server/internal/transport/http/middleware.go` shows the
  pattern for adding HTTP middleware to the mux.
