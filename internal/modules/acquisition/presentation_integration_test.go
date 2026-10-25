package acquisition

import (
	"bytes"
	"testing"
)

func TestDownloadPagePresentationFieldsAreReadOnly(t *testing.T) {
	f := newSourceFixture(t)
	var size int64
	var site string
	if err := f.engine.DB.QueryRow(f.ctx, `SELECT r.size_bytes,i.name FROM resources r JOIN integrations i ON i.id=r.account_id WHERE r.id=$1`, f.resourceID).Scan(&size, &site); err != nil {
		t.Fatal(err)
	}
	var before, after []byte
	if err := f.engine.DB.QueryRow(f.ctx, `SELECT details FROM download_tasks WHERE id=$1`, f.taskID).Scan(&before); err != nil {
		t.Fatal(err)
	}
	page, err := f.engine.Page(f.ctx, 0, 25, "", "", nil)
	if err != nil || len(page) != 1 || page[0].ResourceSize != size || page[0].SiteName != site {
		t.Fatalf("presentation fields: %+v %v", page, err)
	}
	if err = f.engine.DB.QueryRow(f.ctx, `SELECT details FROM download_tasks WHERE id=$1`, f.taskID).Scan(&after); err != nil || !bytes.Equal(before, after) {
		t.Fatal("read changed task", err)
	}
	if _, err = f.engine.DB.Exec(f.ctx, `UPDATE download_tasks SET resource_id=NULL WHERE id=$1`, f.taskID); err != nil {
		t.Fatal(err)
	}
	page, err = f.engine.Page(f.ctx, 0, 25, "", "", nil)
	if err != nil || len(page) != 1 || page[0].ResourceSize != 0 || page[0].SiteName != "" {
		t.Fatal("missing resource must stay unknown", page, err)
	}
}
