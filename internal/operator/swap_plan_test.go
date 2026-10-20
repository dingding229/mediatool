package operator

import (
	"strings"
	"testing"
)

func TestOfflineDatabaseSwapPlanRetainsAtomicQuotedStatements(t *testing.T) {
	next, old := "media_tool_restore_123456abcdef", "media_tool_before_123456abcdef"
	sql, e := databaseSwapSQL("media_tool", next, old)
	if e != nil || sql != `BEGIN;ALTER DATABASE "media_tool" RENAME TO "media_tool_before_123456abcdef";ALTER DATABASE "media_tool_restore_123456abcdef" RENAME TO "media_tool";COMMIT;` {
		t.Fatal(sql, e)
	}
	for _, name := range []string{"", "postgres", "template0", next, old, "name\x00hidden"} {
		if _, e = databaseSwapSQL(name, next, old); e == nil {
			t.Fatal(name)
		}
	}
	value := `owned";DROP DATABASE postgres;--`
	sql, e = databaseSwapSQL(value, next, old)
	if e != nil || strings.Count(sql, `"owned"";DROP DATABASE postgres;--"`) != 2 {
		t.Fatal("identifier escaping", sql, e)
	}
	if _, e = databaseSwapSQL("media_tool", next, "media_tool_restore_123456abcdef"); e == nil {
		t.Fatal("rollback namespace accepted")
	}
}
