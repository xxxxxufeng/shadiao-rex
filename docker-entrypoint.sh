#!/bin/sh
# Bind-mounted ./data is often created root-owned by Docker; take it over,
# then drop to the unprivileged hgsd user.
set -e
if [ "$(id -u)" = 0 ]; then
  chown -R hgsd:hgsd /data
  exec su-exec hgsd hgsd -data /data "$@"
fi
exec hgsd -data /data "$@"
