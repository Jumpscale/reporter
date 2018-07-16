package reporter

import logging "github.com/op/go-logging"

var (
	log = logging.MustGetLogger("reporter")
)

//Recorder interface
type Recorder interface {
	Record(blk *Block) error
	Close() error
}
