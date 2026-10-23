#!/usr/bin/env bash
# Reuse the exact reviewed app's existing FFmpeg, not a new binary provider.
# No credentials, tags, local image archives, app start, or global pruning.
set -euo pipefail
root=/tmp/mediatool-ci-media-tools-f8a
image=dingding229/mediatool@sha256:e458207bb03f850eaf80f828fad1460c1e09f2456903af413b0fc8fb5fb27a3b
[[ $(uname -s) == Linux && $(uname -m) == x86_64 ]]
started=$SECONDS
if [[ ! -f "$root/.complete" ]]; then
  [[ "$root" == /tmp/mediatool-ci-media-tools-f8a ]]
  rm -rf -- "$root"
  mkdir -p "$root/usr/bin" "$root/bin"
  cid=''
  cleanup(){
    if [[ -n "$cid" ]];then
      [[ $(docker inspect --format '{{index .Config.Labels "io.mediatool.ci"}}' "$cid") == media-tools ]] && docker rm "$cid" >/dev/null
    fi
    [[ -f "$root/.complete" ]] || rm -rf -- "$root"
  }
  trap cleanup EXIT
  docker pull --platform linux/amd64 "$image" >/dev/null
  cid=$(docker create --platform linux/amd64 --label io.mediatool.ci=media-tools "$image")
  docker cp "$cid:/lib" "$root/lib"
  docker cp "$cid:/usr/lib" "$root/usr/lib"
  for tool in ffmpeg ffprobe;do
    docker cp "$cid:/usr/bin/$tool" "$root/usr/bin/$tool"
    cat > "$root/bin/$tool" <<WRAPPER
#!/usr/bin/env bash
exec "$root/lib/ld-musl-x86_64.so.1" --library-path "$root/usr/lib:$root/lib" "$root/usr/bin/$tool" "\$@"
WRAPPER
  done
  find "$root" -type d -exec chmod 755 {} +
  find "$root" -type f -exec chmod 644 {} +
  chmod 755 "$root/bin/ffmpeg" "$root/bin/ffprobe" "$root/usr/bin/ffmpeg" "$root/usr/bin/ffprobe" "$root/lib/ld-musl-x86_64.so.1"
  "$root/bin/ffmpeg" -version >/dev/null
  "$root/bin/ffprobe" -version >/dev/null
  printf '%s\n' "$image" > "$root/.complete"
fi
[[ $(cat "$root/.complete") == "$image" ]]
"$root/bin/ffmpeg" -version
"$root/bin/ffprobe" -version
printf '%s\n' "$root/bin" >> "$GITHUB_PATH"
printf 'Pinned cached media tools: %ss (no apt on successful path)\n' "$((SECONDS-started))" >> "$GITHUB_STEP_SUMMARY"
