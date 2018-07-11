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

	rep, err := reporter.NewInfluxReporter("http://localhost:8086/rivine")
	if err != nil {
		panic(err)
	}

	defer rep.Flush()

	scanner := exp.Scan(0)

	for blk := range scanner.Scan(context.Background()) {
		if err := rep.Record(blk); err != nil {
			panic(err)
		}
	}

	fmt.Println(scanner.Err())
}
