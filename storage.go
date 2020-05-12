package files

import (
	"evalgo.org/evmsg"
)

type Storage interface {
	ListBuckets() (*evmsg.Message, error)
}
