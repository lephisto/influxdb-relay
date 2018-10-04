#! /usr/bin/env bash

curl -X POST "http://127.0.0.1:8086/query" --data-urlencode 'q=CREATE DATABASE NOT_prometheus'
curl -X POST "http://127.0.0.1:8086/query" --data-urlencode 'q=CREATE DATABASE prometheus'

# curl -X POST "http://127.0.0.1:8086/query" --data-urlencode 'db=prometheus' --data-urlencode 'q=SHOW SERIES'
# curl -X POST "http://127.0.0.1:8086/query" --data-urlencode 'db=NOT_prometheus' --data-urlencode 'q=SHOW SERIES'

# curl -X POST "http://127.0.0.1:9096/write?db=NOT_prometheus" --data-binary
# 'cpu_load_short,host=server01,region=us-west value=0.64 1434055562000000000'
