package reporter

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	influxdb "github.com/influxdata/influxdb/client/v2"
)

const (
	InfluxDatabaseName   = "rivine"
	InfluxPointBatchSize = 100
	InfluxSeriesName     = "transaction"
)

//Reporter interface
type Reporter interface {
	Record(blk *Block) error
	Flush() error
}

type txnValue struct {
	Output          float64
	Input           float64
	Fees            float64
	InputAddresses  int
	OutputAddresses int
}

type influxReporter struct {
	cl    influxdb.Client
	db    string
	batch influxdb.BatchPoints
}

//NewInfluxReporter creates a new reporter for influxdb
func NewInfluxReporter(u string) (Reporter, error) {
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

	reporter := &influxReporter{cl: cl}
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

	return response.Error()
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

func (r *influxReporter) fields(blk *Block) (map[string]interface{}, error) {
	for _, txn := range blk.Transactions {
		var values struct {
			Output          float64
			Input           float64
			Fees            float64
			InputAddresses  int
			OutputAddresses int
		}
		//updating transaction fees
		for _, fee := range txn.RawTransaction.Data.MinerFees {
			value, err := fee.Float64()
			if err != nil {
				return nil, err
			}

			values.Fees += value
		}

		for _, output := range txn.RawTransaction.Data.CoinOutputs {
			values.OutputAddresses++
			value, err := output.Value.Float64()
			if err != nil {
				return nil, err
			}
			values.Output += value
		}

		for _, input := range txn.CoinInputOutputs {
			values.InputAddresses++
			value, err := input.Value.Float64()
			if err != nil {
				return nil, err
			}
			values.Input += value
		}
	}

	return nil, nil
}

func (r *influxReporter) Record(blk *Block) error {
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
		//todo: should we record transactions with 0 token movement ?
		if values.Output == 0 {
			continue
		}

		point, err := influxdb.NewPoint(
			InfluxSeriesName, nil,
			map[string]interface{}{
				"input":            values.Input,
				"output":           values.Output,
				"input_addresses":  values.InputAddresses,
				"output_addresses": values.OutputAddresses,
				"fees":             values.Fees,
			},
			ts,
		)

		if err != nil {
			return err
		}
		r.batch.AddPoint(point)
	}

	if len(r.batch.Points()) >= InfluxPointBatchSize {
		return r.Flush()
	}

	return nil
}

func (r *influxReporter) Flush() error {
	if err := r.cl.Write(r.batch); err != nil {
		return err
	}

	r.batch = nil
	return nil
}
