package files

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/jpeg"
	"image/png"
	"io/ioutil"
	"mime/multipart"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"evalgo.org/evmsg"
	"github.com/minio/minio-go/v6"
	"github.com/nfnt/resize"

	"log"
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
	mDoneCh := make(chan struct{})
	defer close(doneCh)
	defer close(mDoneCh)
	objects := m.Client.ListObjects(bucket.Name, prefix, true, doneCh)
	msgData := []interface{}{}
	for obj := range objects {
		// TODO that needs an optimization
		metas := m.Client.ListObjects("meta", prefix, true, mDoneCh)
		mObj := map[string]interface{}{}
		if obj.Err != nil {
			msg.Debug.Error = obj.Err.Error()
			return msg, obj.Err
		}
		log.Println("===>", obj.Key, len(metas))
		// TODO this one needs one too
		for meta := range metas {
			if meta.Err != nil {
				msg.Debug.Error = meta.Err.Error()
				return msg, meta.Err
			}
			log.Println(meta.Key, obj.Key, strings.Replace(obj.Key, filepath.Ext(obj.Key), ".json", 1))
			if meta.Key == strings.Replace(obj.Key, filepath.Ext(obj.Key), ".json", 1) {
				msg, err := m.GetObject("meta", meta.Key)
				if err != nil {
					msg.Debug.Error = err.Error()
					return msg, err
				}
				mObj["description"] = msg.Value("description")
			}
		}
		mObj["key"] = obj.Key
		mObj["size"] = obj.Size
		mObj["etag"] = obj.ETag
		mObj["modified"] = obj.LastModified
		mObj["bucket"] = bucket.Name
		log.Println(mObj)
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
	metaFile := strings.Replace(file, filepath.Ext(file), ".json", 1)
	metaFilePath := MinioFilesCacheDir + string(os.PathSeparator) + metaFile
	_, err := os.Stat(cacheFilePath)
	if os.IsNotExist(err) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(MinioDownloadSecondsTimeout)*time.Second)
		defer cancel()
		err := m.Client.FGetObjectWithContext(ctx, "meta", metaFile, metaFilePath, minio.GetObjectOptions{})
		if err != nil {
			msg.Debug.Error = err.Error()
			return msg, err
		}
		err = m.Client.FGetObjectWithContext(ctx, bucket, file, cacheFilePath, minio.GetObjectOptions{})
		if err != nil {
			msg.Debug.Error = err.Error()
			return msg, err
		}
	}
	metaB, err := ioutil.ReadFile(metaFilePath)
	if err != nil {
		msg.Debug.Error = err.Error()
		return msg, err
	}
	meta := map[string]string{}
	err = json.Unmarshal(metaB, &meta)
	if err != nil {
		msg.Debug.Error = err.Error()
		return msg, err
	}
	downloadPath := strings.Replace(MinioDownloadsFilePath, ":bucket", bucket, 1)
	downloadPath = strings.Replace(downloadPath, ":object", fileNameSha, 1)
	msg.Data = []interface{}{
		map[string]interface{}{
			"path":        downloadPath,
			"cached":      cacheFilePath,
			"description": meta["description"],
		},
	}
	return msg, nil
}

func (m *Minio) GetThumbnail(bucket, file string) ([]byte, error) {
	msg, err := m.GetObject(bucket, file)
	if err != nil {
		return nil, err
	}
	resp, err := os.Open(msg.Value("cached").(string))
	if err != nil {
		return nil, err
	}
	image, _, err := image.Decode(resp)
	newImage := resize.Resize(125, 0, image, resize.Lanczos3)
	//newImage := resize.Thumbnail(125, 0, image, resize.Lanczos3)
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

func (m *Minio) CreateBucket(bucket string) (*evmsg.Message, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(MinioConnectionSecondsTimeout)*time.Second)
	defer cancel()
	msg := evmsg.NewMessage()
	msg.State = "Response"
	err := m.Client.MakeBucketWithContext(ctx, bucket, "")
	if err != nil {
		msg.Debug.Error = err.Error()
		return msg, err
	}
	return msg, nil
}

func (m *Minio) BucketExists(bucket string) (*evmsg.Message, error) {
	msg := evmsg.NewMessage()
	msg.State = "Response"
	exists, err := m.Client.BucketExists(bucket)
	if err != nil {
		msg.Debug.Error = err.Error()
		return msg, err
	}
	msg.Data = []interface{}{}
	msg.Data = append(msg.Data.([]interface{}), map[string]interface{}{"exists": exists})
	return msg, nil
}

func (m *Minio) RemoveObject(bucket, file string) (*evmsg.Message, error) {
	msg := evmsg.NewMessage()
	msg.State = "Response"
	err := m.Client.RemoveObject(bucket, file)
	if err != nil {
		msg.Debug.Error = err.Error()
		return msg, err
	}
	err = m.Client.RemoveObject("meta", strings.Replace(file, filepath.Ext(file), ".json", 1))
	if err != nil {
		msg.Debug.Error = err.Error()
		return msg, err
	}
	msg.Data = []interface{}{map[string]interface{}{"bucket": bucket, "file": file, "deleted": "OK"}}
	hasher := sha1.New()
	hasher.Write([]byte(file))
	fileNameSha := base64.URLEncoding.EncodeToString(hasher.Sum(nil))
	cacheFilePath := MinioFilesCacheDir + string(os.PathSeparator) + fileNameSha
	metaFile := strings.Replace(file, filepath.Ext(file), ".json", 1)
	metaFilePath := MinioFilesCacheDir + string(os.PathSeparator) + metaFile
	os.Remove(cacheFilePath)
	os.Remove(metaFilePath)
	return msg, nil

}
