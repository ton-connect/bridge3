#!/bin/bash
set -e

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-EOSQL
    -- TODO get rid of this file, do migrtions instead
    CREATE SCHEMA IF NOT EXISTS bridge;
    
    DROP TABLE IF EXISTS bridge.messages;
    
    CREATE TABLE bridge.messages
    (
        client_id                 text                 not null,
        event_id                  bigint               not null,
        end_time                  timestamp            not null,
        bridge_message            bytea                not null
    );

    CREATE INDEX IF NOT EXISTS messages_client_id_index
        ON bridge.messages (client_id);
EOSQL
