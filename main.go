package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"

	. "git.vpgrp.io/lsantoni/influxdb-relay/config"
	"git.vpgrp.io/lsantoni/influxdb-relay/relayservice"
)

var (
	configFile = flag.String("config", "", "Configuration file to use")
)

func main() {
	flag.Parse()

	// Configuration file is mandatory
	if *configFile == "" {
		fmt.Fprintln(os.Stderr, "Missing configuration file")
		flag.PrintDefaults()
		os.Exit(1)
	}

	// And it has to be loaded in order to continue
	cfg, err := LoadConfigFile(*configFile)
	if err != nil {
		log.Fatal("Problem loading config file: ", err)
	}

	r, err := relayservice.New(cfg)
	if err != nil {
		log.Fatal(err)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)

	go func() {
		<-sigChan
		r.Stop()
	}()

	log.Println("Starting relays...")
	r.Run()
}
