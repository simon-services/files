package files

import (
	"evalgo.org/evmsg"
	"github.com/minio/minio-go/v6"
)

type Storage interface {
	ListBuckets() (*evmsg.Message, error)
	ListObjects(bucket minio.BucketInfo, prefix string) (*evmsg.Message, error)
}
