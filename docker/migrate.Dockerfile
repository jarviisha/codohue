FROM migrate/migrate:v4.18.1
COPY migrations /migrations
COPY --chmod=755 docker/migrate-entrypoint.sh /entrypoint.sh
ENTRYPOINT ["/entrypoint.sh"]
