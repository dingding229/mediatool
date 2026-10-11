#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
image="${1:?Usage: bash scripts/update-server.sh dingding229/mediatool:sha-<40hex> or @sha256:<64hex>}"
[[ "$image" =~ ^(docker\.io/)?dingding229/mediatool(:sha-[a-f0-9]{40}|@sha256:[a-f0-9]{64})$ ]] || { echo 'Only pinned Docker Hub references are accepted.' >&2;exit 1; }
[[ "$(docker compose -f docker-compose.yml config --services | sort | tr '\n' ' ')" == 'app postgres redis ' ]] || exit 1
exec 9>.update.lock;flock -n 9 || exit 1
docker pull --platform linux/amd64 "$image"
[[ "$(docker image inspect "$image" --format '{{.Os}}/{{.Architecture}}')" == linux/amd64 ]] || exit 1
revision=$(docker image inspect "$image" --format '{{index .Config.Labels "org.opencontainers.image.revision"}}')
if [[ "$image" == *:sha-* ]];then [[ "${image##*:sha-}" == "$revision" ]] || { echo 'Image revision mismatch.' >&2;exit 1; };fi
backup="private/update-$(date -u +%Y%m%dT%H%M%SZ)";mkdir -p "$backup";chmod 700 private "$backup"
cp docker-compose.yml "$backup/";chmod 600 "$backup/docker-compose.yml"
docker compose exec -T postgres sh -c 'exec pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB"' | gzip > "$backup/database.sql.gz"
chmod 600 "$backup/database.sql.gz";gzip -t "$backup/database.sql.gz";sha256sum "$backup/database.sql.gz" > "$backup/database.sha256"
# Editing just the app image leaves all direct parameters, mounts and keys intact.
python3 - "$image" <<'PY'
import os,sys,re
from pathlib import Path
p=Path('docker-compose.yml');text=p.read_text();replacement=sys.argv[1]
text,count=re.subn(r'(?m)^(\s{4}image:\s*)[^\n]+',lambda m:m[1]+replacement,text,count=1)
if count!=1:raise SystemExit('Expected first service app image line; no config changed.')
temp=p.with_name('.docker-compose.update');fd=os.open(temp,os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o600)
with os.fdopen(fd,'w')as f:f.write(text);f.flush();os.fsync(f.fileno())
os.replace(temp,p)
PY
if ! docker compose up -d --no-deps --wait --wait-timeout 180 app;then
 echo "Update not healthy. Backup retained at $backup. No automatic schema downgrade or volume deletion." >&2;exit 1
fi
docker compose ps app postgres redis
printf 'Docker Hub image installed: %s\nBackup: %s\n' "$image" "$backup"
