package files

import (
	"context"
	"evalgo.org/evmsg"
	"github.com/minio/minio-go/v6"
	"net/url"
	"os"
	"time"
)

var MinioConnectionSecondsTimeout int64 = 5
var MinioDownloadSecondsTimeout int64 = 100
var MinioFilesCacheDir string = "/tmp/files/minio/cache"
var MinioDownloadsFilePath string = "/v0.0.1/files"

type Minio struct {
	ApiURL       string
	AccessKey    string
	AccessSecret string
	Client       *minio.Client
	SSL          bool
}

func NewMinio() *Minio {
	return &Minio{}
}

func (m *Minio) InitCache() error {
	return os.MkdirAll(MinioFilesCacheDir, 0777)
}

func (m *Minio) Connect(apiURL, accessKey, accessSecret string) error {
	var err error
	var pURL *url.URL
	pURL, err = url.Parse(apiURL)
	if err != nil {
		return err
	}
	if pURL.Scheme == "https" {
		m.SSL = true
	} else {
		m.SSL = false
	}
	m.ApiURL = pURL.Hostname() + ":" + pURL.Port()
	m.AccessKey = accessKey
	m.AccessSecret = accessSecret
	m.Client, err = minio.New(m.ApiURL, m.AccessKey, m.AccessSecret, m.SSL)
	m.Client.TraceOn(nil)
	return err
}

func (m *Minio) ListBuckets() (*evmsg.Message, error) {
	msg := evmsg.NewMessage()
	msg.State = "Response"
	ctx, _ := context.WithTimeout(context.Background(), time.Duration(MinioConnectionSecondsTimeout)*time.Second)
	buckets, err := m.Client.ListBucketsWithContext(ctx)
	if err != nil {
		msg.Debug.Error = err.Error()
		return msg, err
	}
	bMap := []interface{}{}
	for _, bucket := range buckets {
		bucketInfo := map[string]interface{}{
			"name":    bucket.Name,
			"created": bucket.CreationDate,
		}
		bMap = append(bMap, bucketInfo)
	}
	msg.Data = bMap
	return msg, nil

}

func (m *Minio) ListObjects(bucket minio.BucketInfo, prefix string) (*evmsg.Message, error) {
	msg := evmsg.NewMessage()
	msg.State = "Response"
	doneCh := make(chan struct{})
	defer close(doneCh)
	objects := m.Client.ListObjects(bucket.Name, prefix, true, doneCh)
	msgData := []interface{}{}
	for obj := range objects {
		if obj.Err != nil {
			msg.Debug.Error = obj.Err.Error()
			return msg, obj.Err
		}
		mObj := map[string]interface{}{
			"key":      obj.Key,
			"size":     obj.Size,
			"etag":     obj.ETag,
			"modified": obj.LastModified,
		}
		msgData = append(msgData, mObj)
	}
	msg.Data = msgData
	return msg, nil
}

func (m *Minio) GetObject(bucket, file string) (*evmsg.Message, error) {
	msg := evmsg.NewMessage()
	msg.State = "Response"
	cacheFilePath := MinioFilesCacheDir + string(os.PathSeparator) + file
	_, err := os.Stat(cacheFilePath)
	if os.IsNotExist(err) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(MinioDownloadSecondsTimeout)*time.Second)
		defer cancel()
		cacheFilePath := MinioFilesCacheDir + string(os.PathSeparator) + file
		err := m.Client.FGetObjectWithContext(ctx, bucket, file, cacheFilePath, minio.GetObjectOptions{})
		if err != nil {
			msg.Debug.Error = err.Error()
			return msg, err
		}
	}
	msg.Data = []interface{}{
		map[string]interface{}{
			"path": MinioDownloadsFilePath + "/" + file,
		},
	}
	return msg, nil
}
