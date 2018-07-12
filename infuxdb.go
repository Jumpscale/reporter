package reporter

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	influxdb "github.com/influxdata/influxdb/client/v2"
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

//Period look back period
type Period string

//InfluxClient represents a client to influxdb
type InfluxClient struct {
	influxdb.Client
	Database string
}

//NewInfluxDB creates a client connection from url
func NewInfluxDB(u string) (*InfluxClient, error) {
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

	db := strings.Trim(uri.Path, "/")
	//initialize database
	if db == "" {
		db = InfluxDatabaseName
	}

	query := fmt.Sprintf("create database %s", db)
	response, err := cl.Query(influxdb.NewQuery(query, "", ""))
	if err != nil {
		return nil, err
	}

	return &InfluxClient{Client: cl, Database: db}, response.Error()
}

//InfluxQuery struct
type InfluxQuery struct {
	cl *InfluxClient
}

//NewInfluxQuery intialize a new query object
func NewInfluxQuery(cl *InfluxClient) *InfluxQuery {
	return &InfluxQuery{cl: cl}
}

func (q *InfluxQuery) floatValue(response *influxdb.Response, col int) (float64, error) {
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
		return 0, fmt.Errorf("no value")
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

//TransactedToken return transacted tokens in the look back period
func (q *InfluxQuery) TransactedToken(period Period) (float64, error) {
	response, err := q.cl.Query(
		influxdb.NewQuery(
			fmt.Sprintf("SELECT sum(input) as total FROM transaction WHERE time >= now() - %s;", period),
			q.cl.Database,
			"",
		),
	)
	if err != nil {
		return 0, err
	}

	if err := response.Error(); err != nil {
		return 0, err
	}

	return q.floatValue(response, 1)
}

//TotalTokens total tokens on the chain
func (q *InfluxQuery) TotalTokens() (float64, error) {
	/*
		TODO:
		total := amount of coins in genesis coin outputs + (block height - 1) * block reward

		in case of tf:
			block reward = 1
			amount of coins in genesis coin outputs = 695099000

		we need to change this to work with any rivine block chain by making those variables configurable
	*/
	response, err := q.cl.Query(
		influxdb.NewQuery("SELECT 695099000 + last(height) - 1 as total FROM transaction;", q.cl.Database, ""),
	)
	if err != nil {
		return 0, err
	}

	if err := response.Error(); err != nil {
		return 0, err
	}

	return q.floatValue(response, 1)
}
