#!/bin/sh
set -eu
export PGHOST=commerce-db
for db in catalog inventory orders gateway; do
  exists=$(psql -U commerce -d postgres -Atc "SELECT 1 FROM pg_database WHERE datname='${db}'")
  if [ "$exists" != "1" ]; then
    psql -U commerce -d postgres -c "CREATE DATABASE ${db}"
  fi
done
