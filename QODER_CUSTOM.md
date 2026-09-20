# Qoder custom branch

`qoder-custom` adds a Qoder quota provider using CPA's authenticated `/v0/management/plugins/qoder/credits` endpoint. Provider tokens stay in CPA. Teams, dedicated/SOTA and shared credits remain separate, with explicit expiry and unknown-capacity handling. Empty refresh tasks are returned as arrays and tolerated by the frontend.

## Updating

Keep `upstream` pointed at `Willxup/cpa-usage-keeper` and `origin` at this fork. Merge official stable tags into `qoder-custom`; installing an official image bypasses these custom changes.

```sh
git fetch upstream --tags
git switch qoder-custom
git merge <stable-tag>
go test ./internal/quota ./internal/cpa
cd web
npm ci
npm run typecheck
npm test -- src/components/usage/credentials/test/credentialViewModels.test.ts src/components/usage/credentials/test/qoderRefresh.test.ts
npm run build
```

Resolve conflicts, build a new custom Docker image from the repository root, back up the database and Compose file, then replace only the Keeper service. Preserve the `/data` volume, environment, base path and network bindings. Verify single-account and bulk quota refresh without model inference.

Never commit runtime databases, credentials, `.env`, auth files or build caches. A failed merge or check must stop publication/deployment.
