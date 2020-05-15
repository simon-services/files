package files

import (
	"github.com/minio/minio-go/v6"
	"testing"
)

func Test_Unit_ListBuckets(t *testing.T) {
	m := NewMinio()
	err := m.Connect("http://127.0.0.1:9000", "minioadmin", "minioadmin")
	if err != nil {
		t.Error(err)
	}
	_, err = m.ListBuckets()
	if err != nil {
		t.Log(err)
	}
}

func Test_Unit_ListObjects(t *testing.T) {
	m := NewMinio()
	err := m.Connect("http://127.0.0.1:9000", "minioadmin", "minioadmin")
	if err != nil {
		t.Error(err)
	}
	_, err = m.ListObjects(minio.BucketInfo{Name: "test"}, "")
	if err != nil {
		t.Log(err)
	}

}
