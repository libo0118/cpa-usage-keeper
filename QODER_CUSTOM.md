# Qoder custom branch

`qoder-custom` adds a Qoder quota provider using CPA's authenticated `/v0/management/plugins/qoder/credits` endpoint. Provider tokens stay in CPA. Teams, dedicated/SOTA and shared credits remain separate, with explicit expiry and unknown-capacity handling. Empty refresh tasks are returned as arrays and tolerated by the frontend.

Merged upstream stable baseline: `v1.15.5`. Build custom images with a distinguishable version, for example `v1.15.5-qoder.2`.

The credential request drawer also displays Qoder per-request Credits in its existing cost column. Use the matching [CPA core custom branch](https://github.com/libo0118/CLIProxyAPI/tree/qoder-custom), which publishes `qoder_credits` in usage events. Keeper preserves the upstream decimal JSON and optional billing flag in both hot and archive tables. Historical records remain NULL; they are not backfilled as zero.

Explicitly non-billable requests display zero. Billable requests use valid upstream Credits; missing information remains unknown. Verified reductions show a struck-through original value followed by actual consumption, both rounded to two decimals. No USD reference value is invented, and Codex USD calculations remain unchanged.

## Updating

Keep `upstream` pointed at `Willxup/cpa-usage-keeper` and `origin` at this fork. Merge official stable tags into `qoder-custom`; installing an official image bypasses these custom changes.

```sh
git fetch upstream --tags
git switch qoder-custom
git merge <stable-tag>
go test ./...
cd web
npm ci
npm run typecheck
npm test -- src/components/usage/credentials/test/credentialViewModels.test.ts src/components/usage/credentials/test/qoderRefresh.test.ts src/components/usage/credentials/test/qoderCredits.test.ts src/components/usage/credentials/test/CredentialRequestEventsList.test.tsx
npm run build
```

Resolve conflicts, build a new custom Docker image from the repository root, back up the database and Compose file, then replace only the Keeper service. Preserve the `/data` volume, environment, base path and network bindings. Verify single-account and bulk quota refresh without model inference.

Never commit runtime databases, credentials, `.env`, auth files or build caches. A failed merge or check must stop publication/deployment.
