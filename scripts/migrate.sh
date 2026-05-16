#!/bin/bash

set -e

# Load environment variables
source ../.env

# Run all migration files in order
for f in ../migrations/*.up.sql; do
  echo "Running $f..."

  psql "$POSTGRES_DSN" -f "$f"

done

echo "All migrations applied."