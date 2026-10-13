export function tmdbImage(path?: string, size = "w500") {
  return path && /^\/[A-Za-z0-9_-]{1,120}\.(?:jpg|png|webp)$/.test(path)
    ? `/api/v1/metadata/image/${size}/${path.slice(1)}`
    : "";
}
