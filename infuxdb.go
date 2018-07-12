package reporter

import (
	"fmt"
	"net/url"
	"strings"

	influxdb "github.com/influxdata/influxdb/client/v2"
)

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
