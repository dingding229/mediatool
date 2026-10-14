#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
if [[ "${MEDIA_TOOL_TEST_LOCKED:-}" != 1 ]];then exec python3 scripts/test-lock.py "$0" "$@";fi
set -a; source .env; source .env.test; set +a
export DATABASE_URL="$TEST_DATABASE_URL"
python3 - <<'PY'
import os,urllib.parse,socket
if urllib.parse.urlsplit(os.environ['DATABASE_URL']).path!='/media_tool_test':raise SystemExit('E2E may only use media_tool_test, never the business database.')
for port in (18084,18085,18086):
 with socket.socket()as sock:
  try:sock.bind(('127.0.0.1',port))
  except OSError:raise SystemExit('Isolated E2E ports busy; no existing process will be stopped.')
PY
# Always reset only the named isolated database, not the business database.
test_db_exec(){
 if [[ "${TEST_DB_EXEC_MODE:-compose}" == psql ]];then
  psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -c "$1" >/dev/null
 else
  docker exec -i "${TEST_POSTGRES_CONTAINER:-media-tool-postgres-1}" sh -c 'psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d media_tool_test -c "$1"' sh "$1" >/dev/null
 fi
}
go run ./cmd/migrate -test-only -cleanup-test-plugins >/dev/null
test_db_exec 'TRUNCATE site_template_loads,subscriptions,integrations,cloud_files,storage_connections,media_items,media_assets,jobs,outbox_events,administrator_accounts,administrator_auth_limits,media_users,media_user_auth_limits CASCADE'

test_db_exec "INSERT INTO site_metric_schedule(singleton,enabled,cron_expression,revision,next_run_at) VALUES(true,false,'0 * * * *',1,now()+interval '1 hour') ON CONFLICT(singleton) DO UPDATE SET enabled=false,cron_expression='0 * * * *',revision=1,next_run_at=now()+interval '1 hour',last_dispatched_at=NULL"
private=$(mktemp -d)
pids=()
cleanup(){ for pid in "${pids[@]}";do kill "$pid" 2>/dev/null||true;done;for pid in "${pids[@]}";do wait "$pid" 2>/dev/null||true;done;test_db_exec "UPDATE network_proxy_settings SET tmdb_enabled=false,sites_enabled=false,url_ciphertext=''::bytea,revision=1 WHERE id='main'" >/dev/null 2>&1 || true;test_db_exec 'TRUNCATE administrator_accounts,administrator_auth_limits,media_users,media_user_auth_limits CASCADE' >/dev/null 2>&1 || true; rm -rf "$private"; }
trap cleanup EXIT INT TERM
export AUTH_ALLOWED_ORIGINS=https://proxy.fixture.invalid
export APP_ENV=development APP_ADDR=127.0.0.1:18084 APP_PUBLIC_URL=http://localhost:18084 EMBY_GATEWAY_ADDR=127.0.0.1:18085
export APP_SECRET_KEY="$(python3 -c 'import secrets;print(secrets.token_hex(32))')"
export SESSION_SECRET_KEY="$(python3 -c 'import secrets;print(secrets.token_hex(32))')"
export STRM_OUTPUT_DIR="$private/strm"
export APP_LOG_DIR="$private/logs" PASSWORD_RESET_FILE="$private/password" LOCAL_BROWSE_ROOTS="$private/files"
mkdir -p "$STRM_OUTPUT_DIR"
export E2E_FILE_ROOT="$private/files"
mkdir -p "$E2E_FILE_ROOT/Subdirectory"
printf 'Owned local file fixture, not a real movie.\n' > "$E2E_FILE_ROOT/Owned Film.mkv"
printf '1\n00:00:00,000 --> 00:00:01,000\nOwned subtitle fixture\n' > "$E2E_FILE_ROOT/Owned Film.zh-CN.srt"
printf 'Independent file fixture.\n' > "$E2E_FILE_ROOT/Other.mkv"
export CLOUDDRIVE_ENDPOINT='' CLOUDDRIVE_TOKEN='' CLOUDDRIVE_USERNAME='' CLOUDDRIVE_PASSWORD='' EMBY_UPSTREAM_URL='' EMBY_PROXY_ENABLED=false EMBY_302_ENABLED=false
export REDIS_URL="$(python3 -c 'import os,urllib.parse;u=urllib.parse.urlsplit(os.environ["REDIS_URL"]);print(urllib.parse.urlunsplit(u._replace(path="/15")))')"
export E2E_EMBY_ENDPOINT=http://127.0.0.1:18086
python3 scripts/e2e-emby-fixture.py >"$private/provider.log" 2>&1 & pids+=("$!")
export E2E_ISOLATED=1 E2E_BASE_URL=http://localhost:18084 E2E_ADMIN_USERNAME=e2e_admin
export E2E_ADMIN_PASSWORD="$(python3 -c 'import secrets;print("E2E-"+secrets.token_hex(6))')"
go run ./cmd/migrate -test-only >/dev/null
test_db_exec "INSERT INTO native_media_libraries(id,name,media_type)VALUES('movies','电影','movie'),('series','剧集','series')ON CONFLICT(id)DO NOTHING"
go build -o "$private/media-tool" ./cmd/media-tool
"$private/media-tool" -role all >"$private/server.log" 2>&1 & pids+=("$!")

for ((i=0;i<60;i++));do if curl -fsS --max-time 2 "$E2E_BASE_URL/readyz" >/dev/null 2>&1;then break;fi;sleep .5;done
curl -fsS --max-time 2 "$E2E_BASE_URL/readyz" >/dev/null
npm run test:e2e --prefix web -- "$@"
