# Media Tool maintenance rules

- Preserve Go/Vue modular architecture and PostgreSQL/Redis persistence.
- Official deployment uses only the root `docker-compose.yml` and exactly app,
  postgres, redis. All application functions execute inside app. Emby/CloudDrive2
  are existing external dependencies; do not delete their configuration/data.
- For every subsequent update: test, commit and push to dingding229/mediatool,
  wait for its Actions AMD64 Docker Hub publication, then update/test the owned
  server at /root/docker/mediatool using the published immutable Hub image.
  Never substitute local image archives or another registry. Report blockers
  honestly instead of claiming deployment when publication/testing is incomplete.
- Deployment parameters are direct values in the operator-private docker-compose.yml (no .env interpolation). Never commit the initialized Compose or production credentials. Actions credentials stay in Secrets. Per the owner’s explicit 2026-10-12 instruction, authenticated administrator logs/data views show original values and full request/302 URLs. Treat runtime logs and databases as private: never commit them or export secrets into public Actions/release artifacts. Login bodies are not additionally collected. The owner-triggered one-time recovery password remains startup-stdout-only, never copied into the web log store. Do not
  reuse credentials from other projects. Preserve 115 authorization, accounts,
  master key, STRM bodies, media and database volumes.
- Do not run database-mutating integration/E2E suites concurrently. They may use
  only media_tool_test and Redis DB15, never production. Real-media mutations and
  production password reset are not test fixtures.
- STRM contains the stable resolver URL; acquire temporary official 115 links
  using the actual player UA. Preserve strict original-STRM, 302-to-CDN delivery;
  never silently restore media relay, HLS or transcoding.
- Cleanup must be path/label scoped to this program's build residues. No global
  docker prune, no business-volume deletion, and retain recent rollback backups.

- Site templates are compiled Go definitions from the pinned github.com/dingding229/mediatool-site module and load only after administrator selection. On upgrade/startup refresh selected definitions and retain templates used by existing PT accounts; never auto-load the entire catalog. Never download pre-dessert-sites/common inheritance, dynamically compile remote Go, or expose source/commit choices in the template UI. Preserve accounts/credentials when changing definition sources, and invalidate prior definition fingerprints.
