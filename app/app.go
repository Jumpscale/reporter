package app

import (
	"context"

	logging "github.com/op/go-logging"
	"github.com/rivine/reporter"
)

var (
	log = logging.MustGetLogger("reporter.app")
)

//Reporter app
type Reporter struct {
	Height    int64
	Explorer  reporter.Explorer
	Recorders []reporter.Recorder

	cancel context.CancelFunc
}

//Run start collecting and recording statistics data
func (r *Reporter) Run() error {
	log.Infof("scanning block chain starting at height: %d", r.Height)

	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel

	defer func() {
		log.Info("closing recorders")
		for _, recoder := range r.Recorders {
			if err := recoder.Close(); err != nil {
				log.Errorf("recorder close error: %s", err)
			}
		}
	}()

	scanner := r.Explorer.Scan(r.Height)
	for blk := range scanner.Scan(ctx) {
		log.Debugf("processing block %d", blk.Height)
		for _, recorder := range r.Recorders {
			if err := recorder.Record(blk); err != nil {
				log.Errorf("error processing block (%d): %s", blk.Height, err)
				return err
			}
		}
	}

	return scanner.Err()
}

//Stop stops reporter app
func (r *Reporter) Stop() {
	if r.cancel != nil {
		r.cancel()
		r.cancel = nil
	}
}
