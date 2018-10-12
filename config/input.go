package config

// Input type of source
type Input string

const (
	// TypeInfluxdb input type
	TypeInfluxdb   Input = "influxdb"
	// TypePrometheus input type
	TypePrometheus Input = "prometheus"
)
