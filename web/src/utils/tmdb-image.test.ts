import { describe, it, expect } from "vitest";
import { tmdbImage } from "./tmdb-image";
describe("same-origin fixed TMDB images", () => {
  it("uses only a relative image identifier", () =>
    expect(tmdbImage("/owned.jpg")).toBe(
      "/api/v1/metadata/image/w500/owned.jpg",
    ));
  it("rejects arbitrary URLs and path traversal", () => {
    for (const path of [
      "https://evil.invalid/a.jpg",
      "/../private.jpg",
      "//evil/a.jpg",
      "/a.svg",
    ])
      expect(tmdbImage(path)).toBe("");
  });
});
