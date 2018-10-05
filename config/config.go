package config

import (
	"os"

	"github.com/naoina/toml"
)

// Config is an object created from a configuration file
// It is a list of HTTP and/or UDP relays
// Each relay has its own list of backends
type Config struct {
	HTTPRelays []HTTPConfig `toml:"http"`
	UDPRelays  []UDPConfig  `toml:"udp"`
	Verbose    bool
}

// HTTPConfig represents an HTTP relay
type HTTPConfig struct {
	// Name identifies the HTTP relay
	Name string `toml:"name"`

	// Addr should be set to the desired listening host:port
	Addr string `toml:"bind-addr"`

	// Set certificate in order to handle HTTPS requests
	SSLCombinedPem string `toml:"ssl-combined-pem"`

	// Default retention policy to set for forwarded requests
	DefaultRetentionPolicy string `toml:"default-retention-policy"`

	DefaultPingResponse int `toml:"default-ping-response"`

	// Outputs is a list of backed servers where writes will be forwarded
	Outputs []HTTPOutputConfig `toml:"output"`
}

// HTTPOutputConfig represents the specification of an HTTP backend target
type HTTPOutputConfig struct {
	// Name of the backend server
	Name string `toml:"name"`

	// Location should be set to the URL of the backend server's write endpoint
	Location string `toml:"location"`

	// Timeout sets a per-backend timeout for write requests (default: 10s)
	// The format used is the same seen in time.ParseDuration
	Timeout string `toml:"timeout"`

	// Buffer failed writes up to maximum count (default: 0, retry/buffering disabled)
	BufferSizeMB int `toml:"buffer-size-mb"`

	// Maximum batch size in KB (default: 512)
	MaxBatchKB int `toml:"max-batch-kb"`

	// Maximum delay between retry attempts
	// The format used is the same seen in time.ParseDuration (default: 10s)
	MaxDelayInterval string `toml:"max-delay-interval"`

	// Skip TLS verification in order to use self signed certificate
	// WARNING: It's insecure, use it only for developing and don't use in production
	SkipTLSVerification bool `toml:"skip-tls-verification"`

	// Where does the data come from ?
	// This allows us to identify the data in the source code
	// in order to apply a type-based treatment to it
	InputType Input `toml:"type"`
}

// UDPConfig represents a UDP relay
type UDPConfig struct {
	// Name identifies the UDP relay
	Name string `toml:"name"`

	// Addr is where the UDP relay will listen for packets
	Addr string `toml:"bind-addr"`

	// Precision sets the precision of the timestamps (input and output)
	Precision string `toml:"precision"`

	// ReadBuffer sets the socket buffer for incoming connections
	ReadBuffer int `toml:"read-buffer"`

	// Outputs is a list of backend servers where writes will be forwarded
	Outputs []UDPOutputConfig `toml:"output"`
}

// UDPOutputConfig represents the specification of a UDP backend target
type UDPOutputConfig struct {
	// Name identifies the UDP backend
	Name string `toml:"name"`

	// Location should be set to the host:port of the backend server
	Location string `toml:"location"`

	// MTU sets the maximum output payload size, default is 1024
	MTU int `toml:"mtu"`
}

// LoadConfigFile parses the specified file into a Config object
func LoadConfigFile(filename string) (Config, error) {
	var cfg Config

	f, err := os.Open(filename)
	if err != nil {
		return cfg, err
	}
	defer f.Close()

	if err = toml.NewDecoder(f).Decode(&cfg); err == nil {
		for index, relay := range cfg.HTTPRelays {
			for indexB, backend := range relay.Outputs {
				if backend.InputType == "" {
					cfg.HTTPRelays[index].Outputs[indexB].InputType = TypeInfluxdb
				}
			}
		}
	}

	return cfg, err
}
