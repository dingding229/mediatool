import { describe, it, expect } from "vitest";
import { genres, countries, genreChoices, countryChoices } from "./options";
describe("classification checkbox vocabulary", () => {
  it("has canonical country and genre identifiers without duplicates", () => {
    expect(countries.find((c) => c.value === "CN")?.label).toBe("中国");
    expect(countries.find((c) => c.value === "JP")?.label).toBe("日本");
    expect(new Set(countries.map((c) => c.value)).size).toBe(countries.length);
    expect(new Set(genres.map((g) => g.value)).size).toBe(genres.length);
  });
  it("shows existing unknown and cross-type selections without discarding values", () => {
    expect(countryChoices(["ZZ"]).some((c) => c.value === "ZZ")).toBe(true);
    expect(genreChoices("movie", [10764, 555555]).map((g) => g.value)).toEqual(
      expect.arrayContaining([10764, 555555, 28]),
    );
    expect(genreChoices("series", []).some((g) => g.value === 10764)).toBe(
      true,
    );
    expect(genreChoices("series", []).some((g) => g.value === 28)).toBe(false);
  });
});
