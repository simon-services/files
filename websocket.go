package files

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"evalgo.org/evmsg"
	echo "github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	echoLog "github.com/labstack/gommon/log"
	"github.com/minio/minio-go/v6"
	"github.com/neko-neko/echo-logrus/v2/log"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/websocket"
	"io/ioutil"
	"path/filepath"
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
	e.Static("/", webroot)
	e.GET("/v0.0.1/files/buckets/:bucket/objects/:object", func(c echo.Context) error {
		c.Response().WriteHeader(http.StatusOK)
		msg, err := f.WSStorage.GetObject(c.Param("bucket"), c.Param("object"))
		if err != nil {
			return err
		}
		c.Logger().Info(msg.Data.([]interface{})[0].(map[string]interface{})["path"].(string))
		tmpFile := filepath.Base(msg.Data.([]interface{})[0].(map[string]interface{})["path"].(string))
		cacheFilePath := MinioFilesCacheDir + string(os.PathSeparator) + tmpFile
		resp, err := ioutil.ReadFile(cacheFilePath)
		if err != nil {
			return err
		}
		c.Response().Write(resp)
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
