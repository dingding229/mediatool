package ipgeo

import "testing"

func TestChineseCountryDisplay(t *testing.T) {
	for _, tc := range []struct{ raw, want string }{{"US", "美国"}, {"us", "美国"}, {"United States", "美国"}, {"NL", "荷兰"}, {"Netherlands", "荷兰"}, {"United Kingdom", "英国"}, {"South Korea", "韩国"}, {"China", "中国"}, {"中国", "中国"}, {"", ""}, {"Unlisted Country", "Unlisted Country"}} {
		if got := ChineseCountry(tc.raw); got != tc.want {
			t.Fatalf("%q: %q", tc.raw, got)
		}
	}
	for code, name := range countryNames {
		if ChineseCountry(code) != name {
			t.Fatal(code)
		}
	}
	for alias, code := range countryAliases {
		if ChineseCountry(alias) != countryNames[code] {
			t.Fatal(alias)
		}
	}
}
