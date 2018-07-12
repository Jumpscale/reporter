package reporter

import "github.com/tidwall/buntdb"

const (
	BuntdbIndexNames = "unlockhash"
)

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

	return nil, nil
}
