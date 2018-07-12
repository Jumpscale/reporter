package reporter

import (
	"context"
	"fmt"
	"sync"
	"time"

	influxdb "github.com/influxdata/influxdb/client/v2"
)

const (
	InfluxDatabaseName   = "rivine"
	InfluxPointBatchSize = 100
	InfluxSeriesName     = "transaction"
)

type txnValue struct {
	Output          float64
	Input           float64
	Fees            float64
	InputAddresses  int
	OutputAddresses int
}

type InfluxRecorder struct {
	cl            *InfluxClient
	batchSize     int
	batch         influxdb.BatchPoints
	flushInterval time.Duration

	cancel context.CancelFunc
	m      sync.Mutex
}

//NewInfluxRecorder creates a new reporter for influxdb
func NewInfluxRecorder(cl *InfluxClient, batchSize int, flushInterval time.Duration) (Recorder, error) {
	reporter := &InfluxRecorder{cl: cl, batchSize: batchSize, flushInterval: flushInterval}
	return reporter, reporter.init()
}

func (r *InfluxRecorder) init() error {
	//start flusher routine
	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel

	go func(ctx context.Context, d time.Duration) {
		for {
			select {
			case <-time.After(d):
				fmt.Println("timedflush")
				r.flush()
			case <-ctx.Done():
				return
			}
		}
	}(ctx, r.flushInterval)

	return nil
}

func (r *InfluxRecorder) aggregate(txn *Transaction) (txnValue, error) {
	var values txnValue

	//updating transaction fees
	for _, fee := range txn.RawTransaction.Data.MinerFees {
		value, err := fee.Float64()
		if err != nil {
			return values, err
		}

		values.Fees += value
	}

	for _, output := range txn.RawTransaction.Data.CoinOutputs {
		values.OutputAddresses++
		value, err := output.Value.Float64()
		if err != nil {
			return values, err
		}
		values.Output += value
	}

	for _, input := range txn.CoinInputOutputs {
		values.InputAddresses++
		value, err := input.Value.Float64()
		if err != nil {
			return values, err
		}
		values.Input += value
	}

	return values, nil
}

func (r *InfluxRecorder) Record(blk *Block) error {
	r.m.Lock()
	defer r.m.Unlock()

	if r.batch == nil {
		var err error
		r.batch, err = influxdb.NewBatchPoints(influxdb.BatchPointsConfig{Database: r.cl.Database})
		if err != nil {
			return err
		}
	}

	ts := time.Unix(blk.RawBlock.Timestamp, 0)
	for _, txn := range blk.Transactions {
		values, err := r.aggregate(&txn)
		if err != nil {
			return err
		}

		point, err := influxdb.NewPoint(
			InfluxSeriesName, nil,
			map[string]interface{}{
				"input":            values.Input,
				"output":           values.Output,
				"input_addresses":  values.InputAddresses,
				"output_addresses": values.OutputAddresses,
				"fees":             values.Fees,
				"height":           blk.Height,
			},
			ts,
		)

		if err != nil {
			return err
		}
		r.batch.AddPoint(point)
	}

	if len(r.batch.Points()) >= r.batchSize {
		//we already have the lock, then just call _flush
		return r._flush()
	}

	return nil
}

func (r *InfluxRecorder) flush() error {
	r.m.Lock()
	defer r.m.Unlock()
	return r._flush()
}

func (r *InfluxRecorder) _flush() error {
	if r.batch != nil && len(r.batch.Points()) > 0 {
		fmt.Println("flush:", len(r.batch.Points()))
		if err := r.cl.Write(r.batch); err != nil {
			return err
		}

		r.batch = nil
	}

	return nil
}

func (r *InfluxRecorder) Close() error {
	if r.cancel != nil {
		r.cancel()
	}

	return r.flush()
}
