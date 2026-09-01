# Dokploy Gateway Runbook

## Known-good deployment routing

The Go app is served by Dokploy on the container's **internal port `8000`**.

In `docker-compose.yml`, keep this configuration:

```yaml
services:
  web-go:
    expose:
      - "8000"
```

Do **not** add a production `ports:` mapping for `web-go`. Do **not** attach the service to a guessed named external reverse-proxy network. Both changes have caused Dokploy gateway failures.

Dokploy is responsible for routing external traffic to `web-go:8000`.

## Local development

Host bindings belong only in `docker-compose.local.yml`:

```yaml
services:
  web-go:
    ports:
      - "8000:8000"
```

Start the local stack with both files, then verify:

```text
http://127.0.0.1:8000/readyz
```

The endpoint must return HTTP `200` and `{"status":"ready"}`.

## Avoid these regressions

| Change | Why it fails |
| --- | --- |
| Add `ports: - "8000:8000"` to the production Compose file | Conflicts with Dokploy's internal port routing and can result in a public `502 Bad Gateway`. |
| Add `nginx-proxy-manager_default` (or another guessed external network) | Deployment cannot create the app container when that network does not exist. |
| Move the local port mapping out of `docker-compose.local.yml` | Removes the supported local `127.0.0.1:8000` workflow. |

## Gateway troubleshooting order

1. Confirm the deployed container logs show `server starting` on `:8000`.
2. Confirm repeated `GET /healthz` requests return `200`.
3. Confirm production Compose has `expose: 8000` and no `web-go` `ports:` or guessed external proxy network.
4. Redeploy the current `main` revision in Dokploy.
5. If the public URL still returns `502`, check Dokploy's configured application target: it must use the `web-go` service on internal port `8000`.

## Verified reference

The deployment layout was restored in commit `dea7af9` (`Use internal ports for Dokploy routing`).
