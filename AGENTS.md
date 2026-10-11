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
- Credentials stay in .env/Actions Secrets, never source, logs or chat. Do not
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
