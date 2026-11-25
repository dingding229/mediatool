#!/usr/bin/env bash
# CI or the server's isolated replay.sh --with-dind runner only.
set -euo pipefail
cd "$(dirname "$0")/.."
source scripts/test-common.sh
if [[ ${GITHUB_ACTIONS:-} != true ]]; then
  [[ ${DOCKER_HOST:-} == tcp://dind:2376 && ${DOCKER_TLS_VERIFY:-} == 1 && ${DOCKER_CERT_PATH:-} == /certs/client ]] || {
    echo 'Real web restore needs the isolated replay.sh --with-dind daemon' >&2; exit 1;
  }
fi
if [[ ${MEDIA_TOOL_TEST_LOCKED:-} != 1 ]]; then
  devtool test-lock bash "$0" "$@"
  exit "$?"
fi

work=$(mktemp -d)
run="rp33c-$(date -u +%Y%m%d%H%M%S)-$$"
label=com.mediatool.test.run
net="$run"
containers=()
volumes=()
cleanup() {
  local status=$? item
  trap - EXIT
  set +e
  for item in "${containers[@]}"; do
    if [[ $(docker inspect -f '{{index .Config.Labels "com.mediatool.test.run"}}' "$item" 2>/dev/null) == "$run" ]]; then
      docker rm -f "$item" >/dev/null || status=1
    fi
  done
  if [[ $(docker network inspect -f '{{index .Labels "com.mediatool.test.run"}}' "$net" 2>/dev/null) == "$run" ]]; then
    docker network rm "$net" >/dev/null || status=1
  fi
  for item in "${volumes[@]}"; do
    if [[ $(docker volume inspect -f '{{index .Labels "com.mediatool.test.run"}}' "$item" 2>/dev/null) == "$run" ]]; then
      docker volume rm "$item" >/dev/null || status=1
    fi
  done
  rm -rf "$work"
  exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
CGO_ENABLED=0 go test -c -o "$work/restore.test" ./internal/modules/maintenance
docker network create --internal --label "$label=$run" "$net" >/dev/null

# PostgreSQL and the test binary share the same disposable container. No host
# socket is mounted, no port is published, and all candidate databases stay here.
for pre98 in 1 0; do
  pg="$run-$pre98"
  containers+=("$pg")
  volumes+=("$pg")
  docker volume create --label "$label=$run" "$pg" >/dev/null
  docker run -d --name "$pg" --label "$label=$run" --network "$net" \
    --mount "type=volume,src=$pg,dst=/var/lib/postgresql" \
    -e POSTGRES_USER=fixture -e POSTGRES_PASSWORD=fixture-only -e POSTGRES_DB=media_tool_test \
    postgres:18-alpine@sha256:77f585114c32fbca283dc835b0596f4e52b51b4c6662d7810b2f4084f60a1873 \
    postgres -c "mediatool.test_run=$run" >/dev/null
  for _ in $(seq 1 90); do
    if docker exec "$pg" pg_isready -h 127.0.0.1 -U fixture -d media_tool_test >/dev/null; then break; fi
    sleep 1
  done
  docker exec "$pg" pg_isready -h 127.0.0.1 -U fixture -d media_tool_test >/dev/null
  [[ $(docker inspect -f '{{len .HostConfig.PortBindings}}' "$pg") == 0 ]]
  docker cp "$work/restore.test" "$pg:/tmp/restore.test"
  docker exec --user postgres \
    -e MEDIA_TOOL_TEST_LOCKED=1 -e "MEDIA_TOOL_RP33C_TEST_RUN=$run" -e "MEDIA_TOOL_RP33C_PRE98=$pre98" \
    -e 'TEST_DATABASE_URL=postgres://fixture:fixture-only@127.0.0.1:5432/media_tool_test?sslmode=disable' \
    "$pg" /tmp/restore.test -test.v -test.timeout=8m -test.run '^TestRP33CLiveWebRestoreAndCompensation$'
  if [[ $pre98 == 1 ]]; then
    # Confirm the exact known root cause twice; never print arbitrary PG logs.
    hits=$(docker logs "$pg" 2>&1 | grep -c 'ERROR: .*function native_queue_asset_assignment(uuid) does not exist')
    [[ $hits == 2 ]] || { echo 'Both real COPY operations must report the expected missing function' >&2; exit 1; }
    echo 'pre-98 control: PostgreSQL reported the missing assignment function for both imports'
  fi
done
