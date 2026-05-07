FROM migrate/migrate:v4.18.1 AS migrate-bin

FROM postgres:16-alpine

COPY --from=migrate-bin /usr/local/bin/migrate /usr/local/bin/migrate
COPY migrations /migrations
COPY docker/migrate-entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

ENTRYPOINT ["/entrypoint.sh"]
