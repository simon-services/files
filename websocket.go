package files

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	//"crypto/subtle"
	"io/ioutil"
	"path/filepath"

	"evalgo.org/evmsg"
	echo "github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	echoLog "github.com/labstack/gommon/log"
	"github.com/minio/minio-go/v6"
	"github.com/neko-neko/echo-logrus/v2/log"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/websocket"
)

type Files struct {
	WSAddress string
	WSClient  string
	WSSecret  string
	WSWebroot string
	WSStorage Storage
}

func New() *Files {
	return &Files{}
}

func (f *Files) ConnectStorage(sType string, connInfo map[string]string) error {
	switch sType {
	case "minio":
		m := NewMinio()
		err := m.Connect(connInfo["url"], connInfo["key"], connInfo["secret"])
		if err != nil {
			return err
		}
		fmt.Println(err, m)
		f.WSStorage = m
		return nil
	}
	return errors.New("the given storage type <" + sType + "> is not supported!")
}

func (f *Files) Start(address, client, secret, webroot string) error {
	f.WSAddress = address
	f.WSClient = client
	f.WSSecret = secret
	f.WSWebroot = webroot
	evmsg.ID = client
	evmsg.Secret = secret
	e := echo.New()
	log.Logger().SetOutput(os.Stdout)
	log.Logger().SetLevel(echoLog.INFO)
	log.Logger().SetFormatter(&logrus.JSONFormatter{TimestampFormat: time.RFC3339})
	e.Logger = log.Logger()
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	/*e.Use(middleware.BasicAuth(
		func(username, password string, c echo.Context) (bool, error) {
			if subtle.ConstantTimeCompare([]byte(username), []byte("files")) == 1 &&
				subtle.ConstantTimeCompare([]byte(password), []byte("secret")) == 1 {
				return true, nil
			}
			return false, nil
		}),
	)*/
	e.Static("/", webroot)
	e.GET("/v0.0.1/files/buckets/:bucket/objects/:object", func(c echo.Context) error {
		msg, err := f.WSStorage.GetObject(c.Param("bucket"), c.Param("object"))
		if err != nil {
			c.Response().WriteHeader(http.StatusInternalServerError)
			c.Response().Write([]byte(err.Error()))
			return err
		}
		c.Logger().Info(msg.Data.([]interface{})[0].(map[string]interface{})["path"].(string))
		tmpFile := filepath.Base(msg.Data.([]interface{})[0].(map[string]interface{})["path"].(string))
		cacheFilePath := MinioFilesCacheDir + string(os.PathSeparator) + tmpFile
		resp, err := ioutil.ReadFile(cacheFilePath)
		if err != nil {
			c.Response().WriteHeader(http.StatusInternalServerError)
			c.Response().Write([]byte(err.Error()))
			return err
		}
		c.Response().WriteHeader(http.StatusOK)
		c.Response().Write(resp)
		return nil
	})
	e.POST("/v0.0.1/files/buckets/:bucket/objects", func(c echo.Context) error {
		file, err := c.FormFile("file")
		if err != nil {
			return err
		}
		switch strings.ToLower(filepath.Ext(file.Filename)) {
		case ".jpg", ".jpeg", ".png":
			// found the right extensions that are supported
		default:
			// everything else is not supported
			err = errors.New("the given extension <" + strings.ToLower(filepath.Ext(file.Filename)) + "> is not supported!")
			c.Response().WriteHeader(http.StatusInternalServerError)
			c.Response().Header().Set("Content-Type", "application/json")
			msg := evmsg.NewMessage()
			msg.State = "Response"
			msg.Debug.Error = err.Error()
			mB, _ := json.Marshal(msg)
			c.Response().Write(mB)
			if err != nil {
				return err
			}
			return err
		}
		err = f.WSStorage.PutObject(c.Param("bucket"), file)
		if err != nil {
			return err
		}
		msg, err := f.WSStorage.GetObject(c.Param("bucket"), file.Filename)
		if err != nil {
			return err
		}
		mB, err := json.Marshal(msg)
		if err != nil {
			return err
		}
		c.Response().WriteHeader(http.StatusOK)
		c.Response().Header().Set("Content-Type", "application/json")
		c.Response().Write(mB)
		return nil
	})
	e.GET("/v0.0.1/files/buckets/:bucket/thumbnails/:object", func(c echo.Context) error {
		tBytes, err := f.WSStorage.GetThumbnail(c.Param("bucket"), c.Param("object"))
		if err != nil {
			return err
		}
		c.Response().Write(tBytes)
		return nil
	})
	e.GET("/v0.0.1/ws", func(c echo.Context) error {
		s := websocket.Server{
			Handler: websocket.Handler(func(ws *websocket.Conn) {
				defer ws.Close()
			WEBSOCKET:
				for {
					var msg evmsg.Message
					err := websocket.JSON.Receive(ws, &msg)
					if err != nil {
						c.Logger().Error(err)
						if err == io.EOF {
							c.Logger().Info("websocket client closed connection!")
							return
						}
					}
					err = evmsg.Auth(&msg)
					if err != nil {
						c.Logger().Error(err)
						err = websocket.JSON.Send(ws, &msg)
						if err != nil {
							c.Logger().Error(err)
						}
						continue WEBSOCKET
					}
					switch msg.Scope {
					case "Object":
						msg.State = "Response"
						switch msg.Command {
						case "get":
							err = evmsg.CheckRequiredKeys(&msg, []string{"bucket", "file"})
							if err != nil {
								c.Logger().Error(err)
								msg.Debug.Error = err.Error()
							} else {
								nMsg, err := f.WSStorage.GetObject(msg.Value("bucket").(string), msg.Value("file").(string))
								if err != nil {
									c.Logger().Error(err)
								}
								msg = *nMsg
							}
						case "getList":
							err = evmsg.CheckRequiredKeys(&msg, []string{"bucket", "prefix"})
							if err != nil {
								c.Logger().Error(err)
								msg.Debug.Error = err.Error()
							} else {
								nMsg, err := f.WSStorage.ListObjects(minio.BucketInfo{Name: msg.Value("bucket").(string)}, msg.Value("prefix").(string))
								if err != nil {
									c.Logger().Error(err)
								}
								msg = *nMsg
							}

						}

					case "Bucket":
						msg.State = "Response"
						switch msg.Command {
						case "create":
							err = evmsg.CheckRequiredKeys(&msg, []string{"bucket"})
							if err != nil {
								c.Logger().Error(err)
								msg.Debug.Error = err.Error()
							} else {
								nMsg, err := f.WSStorage.CreateBucket(msg.Value("bucket").(string))
								if err != nil {
									c.Logger().Error(err)
								}
								msg = *nMsg
							}
						case "getList":
							nMsg, err := f.WSStorage.ListBuckets()
							if err != nil {
								c.Logger().Error(err)
								msg.Debug.Error = err.Error()
							} else {
								msg = *nMsg
							}
						}
					}
					// send msg response
					err = websocket.JSON.Send(ws, &msg)
					if err != nil {
						c.Logger().Error(err)
					}
				}
			}),
			Handshake: func(*websocket.Config, *http.Request) error {
				return nil
			},
		}
		s.ServeHTTP(c.Response(), c.Request())
		return nil
	})
	return e.Start(address)
}
