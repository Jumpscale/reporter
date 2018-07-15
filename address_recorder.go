package reporter

import (
	"database/sql"
	"fmt"

	_ "github.com/mattn/go-sqlite3"
)

const (
	BuntdbIndexNames = "unlockhash"

	opAdd = 1
	opSub = -1
)

type Addresses map[string]float64

type AddressRecorder struct {
	db *sql.DB
}

func NewAddressRecorder(p string) (*AddressRecorder, error) {
	db, err := sql.Open("sqlite3", p)
	if err != nil {
		return nil, err
	}
	exec := `
	create table if not exists unlockhash (
		address text not null primary key,
		value real
	);

	create index if not exists add_index on unlockhash (address);
	create index if not exists value_index on unlockhash (value);
	`
	_, err = db.Exec(exec)
	if err != nil {
		return nil, err
	}

	return &AddressRecorder{db: db}, nil
}

func (r *AddressRecorder) processInputOutputs(addresses Addresses, i []InputOutput, op float64) error {
	for _, inout := range i {
		delta, err := inout.Value.Float64()
		if err != nil {
			return err
		}

		addresses[inout.UnlockHash] += op * delta
	}

	return nil
}

func (r *AddressRecorder) aggregate(addresses Addresses, txn *Transaction) error {
	//updating transaction fees

	if err := r.processInputOutputs(addresses, txn.RawTransaction.Data.CoinOutputs, opAdd); err != nil {
		return err
	}

	if err := r.processInputOutputs(addresses, txn.CoinInputOutputs, opSub); err != nil {
		return err
	}

	return nil
}

//Get tokens on this address
func (r *AddressRecorder) Get(address string) (float64, error) {
	row := r.db.QueryRow("select value from unlockhash where address = ?;", address)
	var value float64
	if err := row.Scan(&value); err == sql.ErrNoRows {
		return 0, nil
	} else if err != nil {
		return 0, err
	}

	return value, nil
}

func (r *AddressRecorder) set(address string, value float64) error {
	_, err := r.db.Exec("insert or replace into unlockhash (address, value) values (?, ?);", address, value)
	return err
}

//Record record a block on the address recorder
func (r *AddressRecorder) Record(blk *Block) error {
	addresses := Addresses{}

	//add miner fees
	if err := r.processInputOutputs(addresses, blk.RawBlock.MinerPayouts, opAdd); err != nil {
		return err
	}

	for _, txn := range blk.Transactions {
		if err := r.aggregate(addresses, &txn); err != nil {
			return err
		}
	}

	for add, delta := range addresses {
		current, err := r.Get(add)
		if err != nil {
			return err
		}

		if err := r.set(add, current+delta); err != nil {
			return err
		}
	}

	return nil
}

//Close the recorder, any calls to record after that will fail
func (r *AddressRecorder) Close() error {
	return r.db.Close()
}

func (r *AddressRecorder) Addresses(over float64, page, size int) error {
	rows, err := r.db.Query("select address, value from unlockhash where value >= ? order by value desc limit ? offset ?;", over, size, page*size)
	if err != nil {
		return err
	}

	defer rows.Close()

	for rows.Next() {
		var address string
		var value float64
		if err := rows.Scan(&address, &value); err != nil {
			return err
		}

		fmt.Printf("Key: %s Value: %f\n", address, value)
	}

	return nil
}
