package files

import "testing"

func Test_Unit_Minio(t *testing.T) {
	m := NewMinio()
	err := m.Connect("http://127.0.0.1:9000", "minioadmin", "minioadmin")
	if err != nil {
		t.Error(err)
	}
	t.Log(m.ListBuckets())
}
