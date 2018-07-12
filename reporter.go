package reporter

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	influxdb "github.com/influxdata/influxdb/client/v2"
)

const (
	InfluxDatabaseName   = "rivine"
	InfluxPointBatchSize = 100
	InfluxSeriesName     = "transaction"
)

//Recorder interface
type Recorder interface {
	Record(blk *Block) error
	Close() error
}

type txnValue struct {
	Output          float64
	Input           float64
	Fees            float64
	InputAddresses  int
	OutputAddresses int
}

type influxReporter struct {
	cl            influxdb.Client
	db            string
	batchSize     int
	batch         influxdb.BatchPoints
	flushInterval time.Duration

	cancel context.CancelFunc
	m      sync.Mutex
}

//NewInfluxRecorder creates a new reporter for influxdb
func NewInfluxRecorder(u string, batchSize int, flushInterval time.Duration) (Recorder, error) {
	uri, err := url.Parse(u)
	if err != nil {
		return nil, err
	}

	var config influxdb.HTTPConfig
	if uri.User != nil {
		config.Username = uri.User.Username()
		if password, ok := uri.User.Password(); ok {
			config.Password = password
		}
	}

	config.Addr = fmt.Sprintf("%s://%s", uri.Scheme, uri.Host)
	cl, err := influxdb.NewHTTPClient(config)
	if err != nil {
		return nil, err
	}

	reporter := &influxReporter{cl: cl, batchSize: batchSize, flushInterval: flushInterval}
	return reporter, reporter.init(strings.Trim(uri.Path, "/"))
}

func (r *influxReporter) init(db string) error {
	//initialize database
	if db == "" {
		db = InfluxDatabaseName
	}
	r.db = db
	query := fmt.Sprintf("create database %s", db)
	response, err := r.cl.Query(influxdb.NewQuery(query, "", ""))
	if err != nil {
		return err
	}

	if err := response.Error(); err != nil {
		return err
	}

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

func (r *influxReporter) aggregate(txn *Transaction) (txnValue, error) {
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

func (r *influxReporter) Record(blk *Block) error {
	r.m.Lock()
	defer r.m.Unlock()

	if r.batch == nil {
		var err error
		r.batch, err = influxdb.NewBatchPoints(influxdb.BatchPointsConfig{Database: r.db})
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

func (r *influxReporter) flush() error {
	r.m.Lock()
	defer r.m.Unlock()
	return r._flush()
}

func (r *influxReporter) _flush() error {
	if r.batch != nil && len(r.batch.Points()) > 0 {
		fmt.Println("flush:", len(r.batch.Points()))
		if err := r.cl.Write(r.batch); err != nil {
			return err
		}

		r.batch = nil
	}

	return nil
}

func (r *influxReporter) Close() error {
	if r.cancel != nil {
		r.cancel()
	}

	return r.flush()
}
