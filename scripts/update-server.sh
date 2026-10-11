#!/usr/bin/env bash
# Run on the Docker host in the deployment directory after initial migration.
# Only a published immutable Docker Hub image may be installed.
set -euo pipefail
cd "$(dirname "$0")/.."
image="${1:?Usage: bash scripts/update-server.sh dingding229/mediatool:sha-<40hex> or @sha256:<64hex>}"
if [[ ! "$image" =~ ^(docker\.io/)?dingding229/mediatool(:sha-[a-f0-9]{40}|@sha256:[a-f0-9]{64})$ ]];then
 echo 'Refusing mutable tags, non-Docker-Hub registries or other repositories.' >&2;exit 1
fi
[[ -f .env && -f docker-compose.yml ]] || exit 1
[[ "$(docker compose --env-file .env config --services | sort | tr '\n' ' ')" == 'app postgres redis ' ]] || { echo 'Expected exactly app/postgres/redis.' >&2;exit 1; }
exec 9>.update.lock
flock -n 9 || { echo 'Another update is active.' >&2;exit 1; }
APP_IMAGE="$image" docker compose --env-file .env pull app
[[ "$(docker image inspect "$image" --format '{{.Os}}/{{.Architecture}}')" == linux/amd64 ]] || { echo 'Wrong image platform.' >&2;exit 1; }
stamp=$(date -u +%Y%m%dT%H%M%SZ)
backup="private/update-$stamp"
mkdir -p "$backup";chmod 700 private "$backup"
cp .env docker-compose.yml "$backup/";chmod 600 "$backup/.env"
docker compose --env-file .env exec -T postgres sh -c 'exec pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB"' | gzip > "$backup/database.sql.gz"
chmod 600 "$backup/database.sql.gz"
gzip -t "$backup/database.sql.gz"
sha256sum "$backup/database.sql.gz" > "$backup/database.sha256"
python3 - "$image" <<'PY'
from pathlib import Path
import os,sys
p=Path('.env');text=p.read_text();lines=[line for line in text.splitlines() if not line.startswith('APP_IMAGE=')];lines.append('APP_IMAGE='+sys.argv[1]);temp=p.with_name('.env.update');fd=os.open(temp,os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o600)
with os.fdopen(fd,'w') as f:f.write('\n'.join(lines)+'\n');f.flush();os.fsync(f.fileno())
os.replace(temp,p)
PY
if ! docker compose --env-file .env up -d --no-deps --wait --wait-timeout 180 app;then
 echo "Update did not become healthy. Retained backup: $backup. No automatic schema downgrade or business-volume deletion." >&2
 echo 'Restore prior APP_IMAGE only after checking migration compatibility; do not restore a database over newer writes.' >&2
 exit 1
fi
docker compose --env-file .env ps
printf 'Docker Hub image installed: %s\nBackup: %s\n' "$image" "$backup"
