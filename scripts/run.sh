#!/bin/bash

set -ex

# Build app
go build -o app .

export PSQL_HOST="localhost"
export PSQL_PORT="5432"
export PSQL_USER="validator"
export PSQL_PASSWORD="val1dat0r"
export PSQL_DB_NAME="project-sem-1"

# Start app
./app &
