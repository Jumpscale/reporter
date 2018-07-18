package reporter

import (
	"database/sql"
	"encoding/json"
	"fmt"

	_ "github.com/mattn/go-sqlite3"
)

const (
	BuntdbIndexNames = "unlockhash"

	opAdd = 1
	opSub = -1
)

type Address struct {
	Address string
	Tokens  float64
}

func (a Address) MarshalJSON() (text []byte, err error) {
	m := [2]interface{}{a.Address, a.Tokens}
	return json.Marshal(m)
}

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

func (r *AddressRecorder) unlockHashes(c *Condition) ([]string, error) {
	var hashes []string
	switch c.Type {
	case UnlockHashCondition:
		data := c.UnlockHashData()
		hashes = append(hashes, data.UnlockHash)
	case TimeLockCondition:
		data := c.TimeLockData()
		subHashes, err := r.unlockHashes(&data.Condition)
		if err != nil {
			return nil, err
		}
		hashes = append(hashes, subHashes...)
	case AtomicSwapCondition:
		/*
			Atomic swap always come in 2 transactions. The first one (this one here)
			defines the potential addresses that can receive the fund (source and dest)
			then followed by another one that actually moves the fund to either the source (refund)
			or the dest.

			It means for us we can ignore this atomic swap condition for now, and rely on the second
			transaction to actually do the move.

			Of course during this time, the (fund) is actually locked (not liquid) and we might
			have to process this differently if we need to keep track of the liquid vs non-liquid tokens
		*/
	default:
		return nil, fmt.Errorf("unhandled condition type: %v", c.Type)
	}

	return hashes, nil
}

func (r *AddressRecorder) processInputOutputs(addresses Addresses, i []InputOutput, op float64) error {
	for i, inout := range i {
		var unlockHashes []string
		if len(inout.UnlockHash) != 0 {
			unlockHashes = []string{inout.UnlockHash}
		} else {
			var err error
			unlockHashes, err = r.unlockHashes(&inout.Condition)
			if err != nil {
				return fmt.Errorf("at index (%d): %s", i, err)
			}
		}

		// if len(unlockHashes) == 0 {
		// 	return fmt.Errorf("empty unlock hash")
		// }

		delta, err := inout.Value.Float64()
		if err != nil {
			return err
		}

		for _, hash := range unlockHashes {
			addresses[hash] += op * delta
		}
	}

	return nil
}

func (r *AddressRecorder) aggregate(addresses Addresses, txn *Transaction) error {
	//updating transaction fees

	if err := r.processInputOutputs(addresses, txn.RawTransaction.Data.CoinOutputs, opAdd); err != nil {
		return fmt.Errorf("aggregate coinoutputs: %v", err)
	}

	if err := r.processInputOutputs(addresses, txn.CoinInputOutputs, opSub); err != nil {
		return fmt.Errorf("aggregate inputouts: %v", err)
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
		return fmt.Errorf("process minerfees: %v", err)
	}

	for i, txn := range blk.Transactions {
		if err := r.aggregate(addresses, &txn); err != nil {
			return fmt.Errorf("transaction (%d): %v", i, err)
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

//Addresses returns addresses
func (r *AddressRecorder) Addresses(over float64, page, size int) ([]Address, error) {
	rows, err := r.db.Query("select address, value from unlockhash where value >= ? order by value desc limit ? offset ?;", over, size, page*size)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	var addresses []Address

	for rows.Next() {
		var address Address
		// var address string
		// var value float64
		if err := rows.Scan(&address.Address, &address.Tokens); err != nil {
			return nil, err
		}

		addresses = append(addresses, address)
	}

	return addresses, nil
}
