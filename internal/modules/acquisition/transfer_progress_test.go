package acquisition

import "testing"

func TestTransferProgressDoesNotConfuseInstantTransferAndNetworkBytes(t *testing.T) {
	for _, c := range []struct {
		state, phase       string
		bytes, hash, total int64
		want               int
	}{{"pending", "pending", 0, 0, 100, 0}, {"uploading", "hashing", 0, 50, 100, 17}, {"uploading", "uploading", 50, 100, 100, 55}, {"verified", "verified", 0, 100, 100, 90}, {"imported", "verified", 27, 100, 100, 100}} {
		p, _ := TransferProgress(c.state, c.phase, c.bytes, c.hash, c.total)
		if p != c.want {
			t.Fatal(c, p)
		}
	}
}
