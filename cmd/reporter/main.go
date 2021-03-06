package main

import (
	"fmt"
	"os"
	"os/signal"
	"path"
	"sync"
	"syscall"
	"time"

	"github.com/Jumpscale/reporter"
	"github.com/Jumpscale/reporter/app"
	"github.com/codegangsta/cli"
)

func waitSignal() {
	ch := make(chan os.Signal)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGHUP, syscall.SIGTERM)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range ch {
			return
		}
	}()

	wg.Wait()
	return
}

func action(ctx *cli.Context) error {
	home := ctx.String("home")

	if err := os.MkdirAll(home, 0755); err != nil {
		return err
	}

	exp, err := reporter.NewExplorer(ctx.String("explorer"))
	if err != nil {
		return err
	}

	//test connection
	if _, err := exp.GetBlock(0); err != nil {
		return err
	}

	influx, err := reporter.NewInfluxRecorder(ctx.String("influx"), 200, 10*time.Second)
	if err != nil {
		return err
	}

	//get last reported hight and test connection
	height, err := influx.Height()
	if err != nil {
		return err
	}

	if height > 0 {
		height++
	}

	addrRecder, err := reporter.NewAddressRecorder(path.Join(home, "rivine.db"))
	if err != nil {
		return err
	}

	reporter := app.Reporter{
		Explorer:  exp,
		Recorders: []reporter.Recorder{influx, addrRecder},
		Height:    height,
	}

	api := app.API{
		InfluxRecorder:  influx,
		AddressRecorder: addrRecder,
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		err = reporter.Run()
		wg.Done()
	}()

	go func() {
		if err := api.Run(ctx.GlobalString("listen")); err != nil {
			fmt.Println("api error:", err)
		}
	}()

	waitSignal()
	reporter.Stop()

	wg.Wait()
	return err
}

func main() {
	app := cli.App{
		Name:        "rivine-reporter",
		Version:     "0.1",
		Description: "Collect statistics about rivine addresses and transactions",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  "explorer, e",
				Usage: "Explorer url",
				Value: "http://localhost:23110",
			},
			cli.StringFlag{
				Name:  "influx, i",
				Usage: "Influx database in the form http://host:port/db-name",
				Value: "http://localhost:8086/rivine",
			},
			cli.StringFlag{
				Name:  "home, m",
				Usage: "Home directory of reporter",
				Value: "/var/run/reporter",
			},
			cli.StringFlag{
				Name:  "listen, l",
				Usage: "API listen address",
				Value: "127.0.0.1:9921",
			},
		},

		Action: action,
	}

	app.RunAndExitOnError()
}
