package maintenance

import "testing"

func TestRestoreTOCPreservesOnlyNamedObjectsNotOwners(t *testing.T) {
	names := map[string]bool{"ops_web_restores": true, "administrator_sessions": true, "integrations": true}
	for _, line := range []string{
		"1; 0 0 TABLE DATA public ops_web_restores owned",
		"2; 0 0 FK CONSTRAINT public administrator_sessions administrator_sessions_fk owned",
		"3; 0 0 COMMENT public COLUMN administrator_sessions.ip owned",
		"4; 0 0 SCHEMA - public owned",
	} {
		if !preserveTOCEntry(line, names) {
			t.Fatal("preserved object imported", line)
		}
	}
	for _, line := range []string{
		"5; 0 0 TABLE DATA public audit_logs integrations",
		"6; 0 0 TABLE public ops_restore_commit owned",
		"7; 0 0 CONSTRAINT public jobs jobs_pkey integrations",
	} {
		if preserveTOCEntry(line, names) {
			t.Fatal("business object omitted", line)
		}
	}
}
