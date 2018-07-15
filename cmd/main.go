package main

import (
	"fmt"

	"github.com/rivine/reporter"
)

func main() {
	exp, err := reporter.NewExplorer("http://localhost:23110")
	if err != nil {
		panic(err)
	}

	// cl, err := reporter.NewInfluxDB("http://localhost:8086/rivine")
	// if err != nil {
	// 	panic(err)
	// }

	// rep, err := reporter.NewInfluxRecorder(cl, 200, 30*time.Second)
	// if err != nil {
	// 	panic(err)
	// }

	// defer rep.Close()

	rep, err := reporter.NewAddressRecorder("test.db")
	if err != nil {
		panic(err)
	}

	for i := int64(0); i < 1000; i++ {
		blk, err := exp.GetBlock(i)
		if err != nil {
			panic(err)
		}

		if err := rep.Record(blk); err != nil {
			panic(err)
		}

	}

	fmt.Println(rep.Addresses(710100000000.000000, 0, 3))
}
