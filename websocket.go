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
	e.Static("/", webroot)
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
