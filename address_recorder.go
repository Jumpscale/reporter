package reporter

import (
	"fmt"
	"strconv"

	"github.com/tidwall/buntdb"
)

const (
	BuntdbIndexNames = "unlockhash"

	opAdd = 1
	opSub = -1
)

type Addresses map[string]float64

type AddressRecorder struct {
	db *buntdb.DB
}

func NewAddressRecorder(p string) (*AddressRecorder, error) {
	db, err := buntdb.Open(p)
	if err != nil {
		return nil, err
	}

	err = db.Update(func(tx *buntdb.Tx) error {
		idxes, err := tx.Indexes()
		if err != nil {
			return err
		}

		for _, idx := range idxes {
			if idx == BuntdbIndexNames {
				return nil
			}
		}

		return tx.CreateIndex(BuntdbIndexNames, "*", buntdb.IndexFloat)
	})

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

	return r.db.Update(func(tx *buntdb.Tx) error {
		for add, delta := range addresses {
			currentStr, err := tx.Get(add)
			if err != nil && err != buntdb.ErrNotFound {
				return err
			}
			var current float64
			if len(currentStr) > 0 {
				current, err = strconv.ParseFloat(currentStr, 64)
				if err != nil {
					return err
				}
			}

			_, _, err = tx.Set(add, fmt.Sprint(current+delta), nil)
			return err
		}

		return nil
	})
}

//Close the recorder, any calls to record after that will fail
func (r *AddressRecorder) Close() error {
	return r.db.Close()
}
