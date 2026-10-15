import { describe, it, expect } from "vitest";
import { genres, countries, genreChoices, countryChoices, languages, languageChoices, languageLabel, filterChoices } from "./options";
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

describe("searchable classification choices", () => {
  it("shows Chinese language names, canonical codes and saved unknown values", () => {
    expect(languages.find((language) => language.value === "en")?.label).toBe("英语");
    expect(languages.find((language) => language.value === "cn")?.label).toBe("粤语");
    expect(languageChoices(["yue", "cmn"]).find((language) => language.value === "yue")?.label).toBe("粤语");
    expect(languageLabel("cmn")).toBe("汉语（普通话）");
    expect(languageLabel("zho")).toMatch(/[\u3400-\u9fff]/);
    expect(languageLabel("bad_bad")).toBe("已保存语言 bad_bad");
    expect(languages.find((language) => language.value === "zh")?.label).toContain("汉语");
    expect(new Set(languages.map((language) => language.value)).size).toBe(languages.length);
    expect(languages.every((language) => /[\u3400-\u9fff]/.test(language.label) && !language.label.startsWith("已保存语言"))).toBe(true);
    expect(languageLabel("aa")).toBe("阿法尔语");
    expect(languageChoices(["xyz", "xyz"]).filter((language) => language.value === "xyz")).toEqual([
      { value: "xyz", label: "已保存语言 xyz" },
    ]);
  });
  it("pins selections ahead of unselected choices without mutating the input order or model", () => {
    const options = countryChoices(["ZZ", "US", "CN"]);
    const before = options.map((option) => option.value);
    const selected = ["ZZ", "US", "CN"];
    const sorted = filterChoices(options, selected);
    expect(sorted.slice(0, 3).map((option) => option.value)).toEqual(["CN", "US", "ZZ"]);
    expect(selected).toEqual(["ZZ", "US", "CN"]);
    expect(options.map((option) => option.value)).toEqual(before);
    expect(filterChoices(genreChoices("movie", [99]), [99])[0]?.value).toBe(99);
  });
  it("filters all vocabularies by Chinese names or case-insensitive codes", () => {
    expect(filterChoices(countryChoices([]), [], "  us  ")).toEqual([{ value: "US", label: "美国" }]);
    expect(filterChoices(languageChoices([]), [], "EN")).toContainEqual({ value: "en", label: "英语" });
    expect(filterChoices(languageChoices([]), [], "英语").map((option) => option.value)).toEqual(["en"]);
    expect(filterChoices(genreChoices("movie", []), [], "纪录").map((option) => option.value)).toEqual([99]);
    expect(filterChoices(genreChoices("movie", []), [], "纪录 99").map((option) => option.value)).toEqual([99]);
    expect(filterChoices(countryChoices([]), ["CN"], "不存在的国家")).toEqual([]);
  });
  it("restores the catalog when search is cleared and preserves unmatched selections", () => {
    const selected = ["zh", "xyz"];
    expect(filterChoices(languageChoices(selected), selected, "英语")).toEqual([{ value: "en", label: "英语" }]);
    expect(selected).toEqual(["zh", "xyz"]);
    expect(filterChoices(languageChoices(selected), selected, " ").slice(0, 2).map((option) => option.value)).toEqual(["zh", "xyz"]);
  });
});
