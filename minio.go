package files

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"evalgo.org/evmsg"
	"github.com/minio/minio-go/v6"
	"github.com/nfnt/resize"
	"image"
	"image/jpeg"
	"image/png"
	"mime/multipart"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var MinioConnectionSecondsTimeout int64 = 5
var MinioDownloadSecondsTimeout int64 = 100
var MinioFilesCacheDir string = "/tmp/files/minio/cache"
var MinioDownloadsFilePath string = "/v0.0.1/files/buckets/:bucket/objects/:object"
var MinioUploadSecondsTimeout int64 = 100

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
	hasher := sha1.New()
	hasher.Write([]byte(file))
	fileNameSha := base64.URLEncoding.EncodeToString(hasher.Sum(nil))
	cacheFilePath := MinioFilesCacheDir + string(os.PathSeparator) + fileNameSha
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
	downloadPath := strings.Replace(MinioDownloadsFilePath, ":bucket", bucket, 1)
	downloadPath = strings.Replace(downloadPath, ":object", fileNameSha, 1)
	msg.Data = []interface{}{
		map[string]interface{}{
			"path": downloadPath,
		},
	}
	return msg, nil
}

func (m *Minio) GetThumbnail(bucket, file string) ([]byte, error) {
	msg, err := m.GetObject(bucket, file)
	if err != nil {
		return nil, err
	}
	imgPath := msg.Data.([]interface{})[0].(map[string]interface{})["path"].(string)
	resp, err := os.Open(imgPath)
	if err != nil {
		return nil, err
	}
	image, _, err := image.Decode(resp)
	newImage := resize.Resize(125, 0, image, resize.Lanczos3)
	imgBuffer := bytes.NewBuffer(nil)
	switch strings.ToLower(filepath.Ext(file)) {
	case ".jpg", ".jpeg":
		err := jpeg.Encode(imgBuffer, newImage, nil)
		if err != nil {
			return nil, err
		}
	case ".png":
		err := png.Encode(imgBuffer, newImage)
		if err != nil {
			return nil, err
		}
	}
	return imgBuffer.Bytes(), nil

}

func (m *Minio) PutObject(bucket string, file *multipart.FileHeader) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(MinioUploadSecondsTimeout)*time.Second)
	defer cancel()
	src, err := file.Open()
	if err != nil {
		return err
	}
	defer src.Close()
	_, err = m.Client.PutObjectWithContext(ctx, bucket, file.Filename, src, file.Size, minio.PutObjectOptions{
		ContentType: "application/octet-stream",
	})
	if err != nil {
		return err
	}
	return nil
}
