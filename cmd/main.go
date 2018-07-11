package main

import (
	"context"
	"fmt"

	"github.com/rivine/reporter"
)

func main() {
	exp, err := reporter.NewExplorer("http://localhost:23110")
	if err != nil {
		panic(err)
	}

	scanner := exp.Scan(0)

	for blk := range scanner.Scan(context.Background()) {
		for _, txn := range blk.Transactions {
			for i, inout := range txn.CoinInputOutputs {
				fmt.Print("Height: ", blk.Height, " In Value: ", inout.Value, " Source: ", inout.UnlockHash)
				out := txn.RawTransaction.Data.CoinOutputs[i]
				fmt.Println(" Destination: ", out.UnlockHash, " Out Value: ", out.Value)
			}
		}
	}

	fmt.Println(scanner.Err())
}
