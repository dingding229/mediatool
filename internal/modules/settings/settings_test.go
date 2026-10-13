package settings

import "testing"

func TestDefaultsAndValidation(t *testing.T) {
	v := Defaults()
	if Validate(v) != nil {
		t.Fatal(v)
	}
	for _, change := range []func(*Value){func(v *Value) { v.MovieTemplate = "{unknown}{ext}" }, func(v *Value) { v.SeriesTemplate = "{title}{ext}" }, func(v *Value) { v.MovieTemplate = "../{title}" }, func(v *Value) { v.SearchCacheSeconds = -1 }, func(v *Value) { v.TMDBCacheSeconds = 2592001 }, func(v *Value) { v.EmbyUserID = "../unsafe" }, func(v *Value) { v.SubscriptionCron = "* * * * *" }, func(v *Value) { v.SubscriptionCron = "@every 1h" }} {
		v := Defaults()
		change(&v)
		if Validate(v) == nil {
			t.Fatal(v)
		}
	}
}
