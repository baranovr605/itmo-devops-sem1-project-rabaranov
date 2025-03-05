#!/bin/bash

set -ex

go mod tidy

# If no need create database
if [[ "$1" == "not_create_database" ]];
    then
        exit 0
    fi

# Vars for connect to database
PSQL_HOST="project-sem-1"
PSQL_PORT="5432"
PSQL_USER="validator"
PSQL_PASSWORD="val1dat0r"
PSQL_DB_NAME="project-sem-1"
PSQL_DEFAULT_DB_NAME="posgres"
export PGPASSWORD="$PSQL_PASSWORD"

# Drop database (for clean all data)
psql -U "$PSQL_USER" -h "$PSQL_HOST" -p "$PSQL_PORT" -d "$PSQL_DEFAULT_DB_NAME" -c "DROP DATABASE IF EXISTS \"$PSQL_DB_NAME\";"

# Create database from zero
psql -U "$PSQL_USER" -h "$PSQL_HOST" -p "$PSQL_PORT" -d "$PSQL_DEFAULT_DB_NAME" -c "CREATE DATABASE \"$PSQL_DB_NAME\";"

# Create table for write data
psql -U "$PSQL_USER" -h "$PSQL_HOST" -p "$PSQL_PORT" -d "$PSQL_DB_NAME" -c "
CREATE TABLE IF NOT EXISTS prices (
    id SERIAL PRIMARY KEY,
    name TEXT,
    category TEXT,
    price TEXT,
    create_data TEXT
);"
