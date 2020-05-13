package files

import (
	"evalgo.org/evmsg"
	"github.com/minio/minio-go/v6"
	"mime/multipart"
)

type Storage interface {
	CreateBucket(bucket string) (*evmsg.Message, error)
	ListBuckets() (*evmsg.Message, error)
	ListObjects(bucket minio.BucketInfo, prefix string) (*evmsg.Message, error)
	GetObject(bucket, file string) (*evmsg.Message, error)
	GetThumbnail(bucket, file string) ([]byte, error)
	PutObject(bucket string, file *multipart.FileHeader) error
}
