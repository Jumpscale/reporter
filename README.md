# TF Reporter
The TF reporter follows the threefold block chain (using the explorer) and uses the block transaction data to collect and calculate some useful statistics.

The TF reporter once it catches up with the blocks it will provide the following end points to query.

## API
### GET    /height
Returns the latest block height

### GET    /tokens/total
Calculates the total number of tokens on the network

### GET    /tokens/transacted
Query Params:
```
period=<period>
```
Calculates the total number of transacted tokens on the network on the last period (default is 1 hour)
The period defines the look back time and is always evaluated as `now() - period`

The period syntax is `<number><suffix>` where the suffix is one of the following:
- `s`: Seconds
- `m`: Minutes
- `h`: Hours
- `d`: Days
- `w`: Weeks

We also support `u` for microseconds, and `ms` for milliseconds but don't think they have an actual usage case.

### GET    /address
Query Params:
```
over=<amount> default 0
size=<size> default 20
page=<page> default 0
```

List all addresses sorted in a descending order based on the tokens associated to the address.

`over` if provided filters addresses that has more than (or equal) this amount of tokens, same sorting rule applies.
`size` is the max number of addresses returned by this call, default is page size of 20
`page` 0 index page number, a caller of this endpoint can keep incrementing the page number under he receives a null, or a page with fewer entries than the requested page size

### GET    /address/:address
URL Params:
```
address=<wallet address/unlockhash>
```

return the tracked amount of tokens/fund associated with this address.

## Limitations and Issues
Please not the following known limitations

- There is no distinction between liquid and locked tokens, all transactions are considered immediate.
- Multisegnature transactions assumes the fund has been transferred to *each* potential target address.

## Operation
### Requirements
- rivine/tfchain explorer.
- influxdb

### Installation
```
go get -u github.com/Jumpscale/reporter/cmd/...
```

### Command
```
# reporter -h
NAME:
   rivine-reporter

USAGE:
    [global options] command [command options] [arguments...]

VERSION:
   0.1

DESCRIPTION:
   Collect statistics about rivine addresses and transactions

COMMANDS:
     help, h  Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --explorer value, -e value  Explorer url (default: "http://localhost:23110")
   --influx value, -i value    Influx database in the form http://host:port/db-name (default: "http://localhost:8086/rivine")
   --home value, -m value      Home directory of reporter (default: "/var/run/reporter")
   --listen value, -l value    API listen address (default: "127.0.0.1:9921")
   --help, -h                  show help
   --version, -v               print the version
```

> Please note it's a logical error to change the home of the reporter `-m` after running it for the first time, and keep using the same influxdb instance. The reporter stores some data in influxdb, and other in the sqlite db under the home director `-m`. If the home is changed without making sure influxdb series is dropped, the data will be out of sync and the statistics will be wrong.