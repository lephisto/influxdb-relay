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

const relayVersion = "2.0.0"

var (
	configFile 	= flag.String("config", "", "Configuration file to use")
	verbose    	= flag.Bool("v", false, "If set, InfluxDB Relay will log HTTP requests")
	versionFlag = flag.Bool("version", false, "Print current InfluxDB Relay version")
)

func main() {
	flag.Usage = func() {
		fmt.Println("Please, see README for more information about InfluxDB Relay...\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	if *versionFlag {
		fmt.Println("Influxdb-relay version " + relayVersion)
		os.Exit(0)
	}

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

	cfg.Verbose = *verbose

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
