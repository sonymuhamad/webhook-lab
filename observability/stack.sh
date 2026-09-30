#!/usr/bin/env bash
# Starts, stops, or shows the local observability stack: Prometheus, Loki, and
# Grafana, installed with Homebrew and run as brew services.
#
#   observability/stack.sh up|down|status
#
# Postgres is left out on purpose: the same server holds other databases.
set -euo pipefail

services=(prometheus loki grafana)

case ${1:-status} in
  up)
    for s in "${services[@]}"; do brew services start "$s"; done
    ;;
  down)
    for s in "${services[@]}"; do brew services stop "$s"; done
    ;;
  status)
    brew services list | grep -E "^($(IFS='|'; echo "${services[*]}"))[[:space:]]"
    ;;
  *)
    echo "usage: $0 up|down|status" >&2
    exit 2
    ;;
esac
