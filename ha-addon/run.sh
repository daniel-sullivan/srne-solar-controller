#!/usr/bin/with-contenv bashio
# shellcheck shell=bash

CONFIG="/data/srne.toml"

# --- Server section ---
POLL_INTERVAL=$(bashio::config 'poll_interval')
INGRESS_PORT=$(bashio::addon.ingress_port)

cat > "${CONFIG}" <<EOF
[server]
poll_interval = "${POLL_INTERVAL}s"
web_port = ${INGRESS_PORT}
EOF

# --- Inverter sections ---
for inverter in $(bashio::config 'inverters|keys'); do
    HOST=$(bashio::config "inverters[${inverter}].host")
    PORT=$(bashio::config "inverters[${inverter}].port")
    SLAVE_ID=$(bashio::config "inverters[${inverter}].slave_id")
    SERIAL=$(bashio::config "inverters[${inverter}].serial")

    cat >> "${CONFIG}" <<EOF

[[inverter]]
host = "${HOST}"
port = ${PORT}
slave_id = ${SLAVE_ID}
EOF
    if [ "${SERIAL}" -gt 0 ] 2>/dev/null; then
        echo "serial = ${SERIAL}" >> "${CONFIG}"
    fi
done

# --- MQTT section (required for HA auto-discovery) ---
MQTT_BROKER=""
MQTT_USER=""
MQTT_PASS=""
MQTT_PREFIX="homeassistant"
if bashio::config.has_value 'mqtt_topic_prefix'; then
    MQTT_PREFIX=$(bashio::config 'mqtt_topic_prefix')
fi

# Try HA Supervisor MQTT service first
if bashio::services.available "mqtt"; then
    MQTT_HOST=$(bashio::services mqtt "host")
    MQTT_PORT=$(bashio::services mqtt "port")
    MQTT_BROKER="tcp://${MQTT_HOST}:${MQTT_PORT}"
    MQTT_USER=$(bashio::services mqtt "username")
    MQTT_PASS=$(bashio::services mqtt "password")
    bashio::log.info "Auto-discovered MQTT broker: ${MQTT_BROKER}"
fi

# Manual overrides take precedence
if bashio::config.has_value 'mqtt_broker'; then
    MQTT_BROKER=$(bashio::config 'mqtt_broker')
    bashio::log.info "Using manual MQTT broker: ${MQTT_BROKER}"
fi
if bashio::config.has_value 'mqtt_username'; then
    MQTT_USER=$(bashio::config 'mqtt_username')
fi
if bashio::config.has_value 'mqtt_password'; then
    MQTT_PASS=$(bashio::config 'mqtt_password')
fi

if [ -n "${MQTT_BROKER}" ]; then
    cat >> "${CONFIG}" <<EOF

[mqtt]
broker = "${MQTT_BROKER}"
topic_prefix = "${MQTT_PREFIX}"
EOF
    if [ -n "${MQTT_USER}" ]; then
        echo "username = \"${MQTT_USER}\"" >> "${CONFIG}"
    fi
    if [ -n "${MQTT_PASS}" ]; then
        echo "password = \"${MQTT_PASS}\"" >> "${CONFIG}"
    fi
else
    bashio::log.error "No MQTT broker available. Install the Mosquitto broker add-on or configure a broker URL."
    exit 1
fi

bashio::log.info "Generated config:"
cat "${CONFIG}"
bashio::log.info "Starting SRNE Solar Controller..."

exec /usr/bin/srne serve --config "${CONFIG}"
