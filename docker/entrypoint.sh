#!/bin/sh
set -eu

mkdir -p /app/certs
chown gymbag:gymbag /app/certs

exec setpriv \
  --reuid=gymbag \
  --regid=gymbag \
  --init-groups \
  --inh-caps +net_bind_service \
  --ambient-caps +net_bind_service \
  -- "$@"
