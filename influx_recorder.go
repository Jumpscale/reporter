package reporter

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sync"
	"time"

	influxdb "github.com/influxdata/influxdb/client/v2"
)

const (
	InfluxDatabaseName   = "rivine"
	InfluxPointBatchSize = 100
	InfluxSeriesName     = "transaction"
)

const (
	//LastHour period
	LastHour Period = "1h"
	//LastDay period
	LastDay Period = "1d"
	//LastWeek period
	LastWeek Period = "1w"
	//LastMonth period
	LastMonth Period = "4w"
)

var (
	NoValueError = fmt.Errorf("no value")

	periodP      = regexp.MustCompile(`^\d+(\w{1,2})$`)
	periodSuffix = map[string]struct{}{
		"u":  struct{}{},
		"ms": struct{}{},
		"s":  struct{}{},
		"m":  struct{}{},
		"h":  struct{}{},
		"d":  struct{}{},
		"w":  struct{}{},
	}
)

//Period look back period
type Period string

//Valid validate period string
func (p Period) Valid() error {
	m := periodP.FindStringSubmatch(string(p))
	if len(m) == 0 {
		return fmt.Errorf("invalid period format, expecting <number><suffix>")
	}

	if _, ok := periodSuffix[m[1]]; !ok {
		return fmt.Errorf("invalid period suffix, where suffix is one of (u, ms, s, m, h, d, w)")
	}

	return nil
}

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
func NewInfluxRecorder(u string, batchSize int, flushInterval time.Duration) (*InfluxRecorder, error) {
	cl, err := newInfluxDB(u)
	if err != nil {
		return nil, err
	}
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

//Close the recorder, and flushes any points in buffer
func (r *InfluxRecorder) Close() error {
	if r.cancel != nil {
		r.cancel()
	}

	return r.flush()
}

func (r *InfluxRecorder) floatValue(response *influxdb.Response, col int) (float64, error) {
	var value interface{}
	if len(response.Results) > 0 {
		result := response.Results[0]
		if len(result.Series) > 0 {
			row := result.Series[0]

			if len(row.Values) > 0 {
				values := row.Values[0]
				if col < len(values) {
					value = row.Values[0][col]
				} else {
					return 0, fmt.Errorf("column out of range")
				}
			}
		}
	}

	switch value := value.(type) {
	case nil:
		return 0, NoValueError
	case float64:
		return value, nil
	case int64:
		return float64(value), nil
	case json.Number:
		return value.Float64()
	default:
		return 0, fmt.Errorf("unkown value type '%t' (%v)", value, value)
	}
}

//Height returns the last reported height in the database
func (r *InfluxRecorder) Height() (int64, error) {
	response, err := r.cl.Query(influxdb.NewQuery("select last(height) as height from transaction;", r.cl.Database, ""))
	if err != nil {
		return 0, err
	}

	f, err := r.floatValue(response, 1)
	if err != nil && err != NoValueError {
		return 0, err
	}

	return int64(f), nil
}

//TransactedToken return transacted tokens in the look back period
func (r *InfluxRecorder) TransactedToken(period Period) (float64, error) {
	if err := period.Valid(); err != nil {
		return 0, err
	}

	response, err := r.cl.Query(
		influxdb.NewQuery(
			fmt.Sprintf("SELECT sum(input) as total FROM transaction WHERE time >= now() - %s;", period),
			r.cl.Database,
			"",
		),
	)
	if err != nil {
		return 0, err
	}

	if err := response.Error(); err != nil {
		return 0, err
	}

	return r.floatValue(response, 1)
}

//TotalTokens total tokens on the chain
func (r *InfluxRecorder) TotalTokens() (float64, error) {
	/*
		TODO:
		total := amount of coins in genesis coin outputs + (block height - 1) * block reward

		in case of tf:
			block reward = 1
			amount of coins in genesis coin outputs = 695099000

		we need to change this to work with any rivine block chain by making those variables configurable
	*/
	response, err := r.cl.Query(
		influxdb.NewQuery("SELECT 695099000 + last(height) - 1 as total FROM transaction;", r.cl.Database, ""),
	)
	if err != nil {
		return 0, err
	}

	if err := response.Error(); err != nil {
		return 0, err
	}

	return r.floatValue(response, 1)
}
