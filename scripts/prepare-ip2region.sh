#!/bin/sh
# Build/CI/development only. The application never downloads an IP database.
set -eu
commit=f8b0792b0cd729f5b2593f690aeed225e49c1967
out=${1:?output directory required}
mkdir -p "$out"
verify(){ if command -v sha256sum >/dev/null 2>&1; then printf '%s  %s\n' "$2" "$1" | sha256sum -c - >/dev/null; else printf '%s  %s\n' "$2" "$1" | shasum -a 256 -c - >/dev/null; fi; }
fetch(){
 name=$1; path=$2; digest=$3
 if [ -f "$out/$name" ] && verify "$out/$name" "$digest" 2>/dev/null; then return; fi
 temp=$(mktemp "$out/.ip2region.XXXXXX")
 trap 'rm -f "$temp"' EXIT HUP INT TERM
 curl --fail --location --proto '=https' --tlsv1.2 --retry 3 --max-time 180 "https://raw.githubusercontent.com/lionsoul2014/ip2region/$commit/$path" -o "$temp"
 verify "$temp" "$digest"
 chmod 444 "$temp"
 mv "$temp" "$out/$name"
 trap - EXIT HUP INT TERM
}
fetch ip2region_v4.xdb data/ip2region_v4.xdb 27ecba8c9a6d5e3b280aedbe7ee15c65372b179fbd0dad3845d8c608f2fcace1
fetch ip2region_v6.xdb data/ip2region_v6.xdb 939f6b46bd2b8bec3cf7c5ceb8ba782266ae9b1f35b5ba7916700dec0b7506ed
fetch LICENSE.md LICENSE.md fe01f2f8fcaafac539154e6aa80b0b7f8af54e01dc4d52322f72971991c6280e
printf 'Verified offline ip2region data: %s\n' "$commit"
