# influxdb-relay

[![License][license-img]][license-href]

1. [Overview](#overview)
2. [Description](#description)
3. [Requirements](#requirements)
4. [Setup](#setup)
5. [Usage](#usage)
6. [Limitations](#limitations)
7. [Development](#development)
8. [Miscellaneous](#miscellaneous)

## Overview

Maintained fork of [influxdb-relay][overview-href] originally developed by
InfluxData. Replicate InfluxDB data for high availability.

## Description

This project adds a basic high availability layer to InfluxDB. With the right
architecture and disaster recovery processes, this achieves a highly available
setup.

## Requirements

- [Go](https://golang.org/doc/install) 1.8+

## Setup

### Go

Download the daemon into your `$GOPATH` and install it in `/sur/sbin`.

```sh
go get -u github.com/vente-privee/influxdb-relay
cp ${GOPATH}/bin/influxdb-relay /usr/bin/influxdb-relay
chmod 755 /usr/bin/influxdb-relay
```

Create the configuration file in `/etc/influxdb-relay`.

```sh
mkdir -p /etc/influxdb-relay
cp ${GOPATH}/src/github.com/vente-privee/influxdb-relay/examples/sample.conf \
   /etc/influxdb-relay/influxdb-relay.conf
```

### Docker

Build your own image.

```sh
mkdir -p ${GOPATH}/src/github.com/vente-privee
cd ${GOPATH}/src/github.com/vente-privee
git clone git@github.com:vente-privee/influxdb-relay
docker build \
       --file Dockerfile \
       --rm \
       --tag influxdb-relay-builder:latest \
       .
mkdir -p /etc/influxdb-relay
cp ${GOPATH}/src/github.com/vente-privee/influxdb-relay/examples/sample.conf \
   /etc/influxdb-relay/influxdb-relay.conf
# Edit /etc/influxdb-relay/influxdb-relay.conf
docker run \
       --volume /etc/influxdb-relay/influxdb-relay.conf:/etc/influxdb-relay/influxdb-relay.conf
       --rm
       influxdb-relay-builder:latest
```

or

Docker pull our image.

```sh
docker pull vptech/nfluxdb-relay-builder:latest
mkdir -p /etc/influxdb-relay
cp ${GOPATH}/src/github.com/vente-privee/influxdb-relay/examples/sample.conf \
   /etc/influxdb-relay/influxdb-relay.conf
Edit /etc/influxdb-relay/influxdb-relay.conf
docker run \
       --volume /etc/influxdb-relay/influxdb-relay.conf:/etc/influxdb-relay/influxdb-relay.conf
       --rm
       vptech/nfluxdb-relay-builder:latest
```

## Usage

You can find more documentation in [docs](docs) folder.

* [Architecture](docs/architecture.md)
* [Buffering](docs/buffering.md)
* [Caveats](docs/caveats.md)
* [Recovery](docs/recovery.md)
* [Sharding](docs/sharding.md)

You can find some configurations in [examples](examples) folder.

* [sample.conf](sample.conf)
* [sample_buffered.conf](sample_buffered.conf)
* [kapacitor.conf](kapacitor.conf)

### Configuration

```toml
[[http]]
# Name of the HTTP server, used for display purposes only.
name = "example-http"

# TCP address to bind to, for HTTP server.
bind-addr = "127.0.0.1:9096"

# Ping response code, default is 204
default-ping-response = 200

# Enable HTTPS requests.
ssl-combined-pem = "/path/to/influxdb-relay.pem"

# Array of InfluxDB instances to use as backends for Relay.
output = [
    # name: name of the backend, used for display purposes only.
    # location: full URL of the /write endpoint of the backend
    # timeout: Go-parseable time duration. Fail writes if incomplete in this time.
    # skip-tls-verification: skip verification for HTTPS location. WARNING: it's insecure. Don't use in production.
    # type: type of input source. OPTIONAL: see below for more information.

    # InfluxDB
    { name="local-influxdb01", location="http://127.0.0.1:8086/write", timeout="10s", type="influxdb" },
    { name="local-influxdb02", location="http://127.0.0.1:7086/write", timeout="10s", type="influxdb" },

    # Prometheus
    { name="local-prometheus01", location="http://127.0.0.1:9090/api/v1/prom/write", timeout="10s", type="prometheus" },
    { name="local-prometheus02", location="http://127.0.0.1:8090/api/v1/prom/write", timeout="10s", type="prometheus" },
]

[[udp]]
# Name of the UDP server, used for display purposes only.
name = "example-udp"

# UDP address to bind to.
bind-addr = "127.0.0.1:9096"

# Socket buffer size for incoming connections.
read-buffer = 0 # default

# Precision to use for timestamps
precision = "n" # Can be n, u, ms, s, m, h

# Array of InfluxDB instances to use as backends for Relay.
output = [
    # name: name of the backend, used for display purposes only.
    # location: host and port of backend.
    # mtu: maximum output payload size
    { name="local1", location="127.0.0.1:8089", mtu=512 },
    { name="local2", location="127.0.0.1:7089", mtu=1024 },
]
```

InfluxDB Relay is able to forward from a variety of input sources, including:

* `influxdb`
* `prometheus`

The `type` parameter in the configuration file defaults to `influxdb`.

## Limitations

So far, this is compatible with Debian, RedHat, and other derivatives.

Moreover, the `type` parameter is not supported when it comes to UDP forwarding.

## Development

Please read carefully [CONTRIBUTING.md][contribute-href] before making a merge
request.

Clone repository into your `$GOPATH`.

```sh
mkdir -p ${GOPATH}/src/github.com/vente-privee
cd ${GOPATH}/src/github.com/vente-privee
git clone git@github.com:vente-privee/influxdb-relay
```

Enter the directory and build the daemon.

```sh
cd ${GOPATH}/src/github.com/vente-privee/influxdb-relay
go build -a -ldflags '-extldflags "-static"' -o influxdb-relay
```

## Miscellaneous

```
    ╚⊙ ⊙╝
  ╚═(███)═╝
 ╚═(███)═╝
╚═(███)═╝
 ╚═(███)═╝
  ╚═(███)═╝
   ╚═(███)═╝
```

[license-img]: https://img.shields.io/badge/license-MIT-blue.svg
[license-href]: LICENSE
[overview-href]: https://github.com/influxdata/influxdb-relay
[contribute-href]: CONTRIBUTING.md
