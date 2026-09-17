#!/bin/sh
set -eu

SERVICE_NAME="wirely.service"
SYSTEMD_DIR="/etc/systemd/system"
ADDRESS=":8080"
SECURE_COOKIE="false"
QUEUE_RATE="1s"
API_RATE_LIMIT="120"
BACKUP_INTERVAL="24h"
BACKUP_RETENTION="7"
BUILD="true"
BUILD_ONLY="false"
START_SERVICE="true"
OPEN_FIREWALL="false"
TARGET_USER=""
INSTALL_DIR=""
DATA_DIR=""
MIGRATE_FROM=""
ADDRESS_SET="false"
DATA_DIR_SET="false"
SECURE_COOKIE_SET="false"
BUILD_TMP=""

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)

usage() {
    cat <<'EOF'
Wirely API installer

Usage:
  sudo ./install.sh [options]

Options:
  --user USER              Linux user (default: the user who invoked sudo)
  --install-dir PATH       Install directory (default: /home/USER/wirely)
  --data-dir PATH          Data directory (default: INSTALL_DIR/data)
  --migrate-from PATH      Copy an older data directory into INSTALL_DIR/data
  --address ADDRESS        Listen address (default: :8080)
  --secure-cookie          Require HTTPS for the dashboard session cookie
  --skip-build             Install the existing bin/wirely binary
  --build-only             Build bin/wirely without installing anything
  --no-start               Install without starting the service
  --open-firewall          Allow the configured TCP port using UFW or iptables
  -h, --help               Show this help

The installer keeps the binary, configuration, database, and WhatsApp sessions
inside /home/USER/wirely by default. Existing data is never deleted.
EOF
}

fail() {
    printf 'wirely: %s\n' "$1" >&2
    exit 1
}

cleanup() {
    if [ -n "$BUILD_TMP" ] && [ -d "$BUILD_TMP" ]; then
        rm -rf -- "$BUILD_TMP"
    fi
}

version_at_least() {
    lowest=$(printf '%s\n%s\n' "$1" "$2" | sort -V | head -n 1)
    [ "$lowest" = "$2" ]
}

validate_path() {
    path_to_validate=$1
    case "$path_to_validate" in
        /*) ;;
        *) fail "path must be absolute: $path_to_validate" ;;
    esac
    case "$path_to_validate" in
        "/"|"/home"|"/root"|"/usr"|"/var"|*[" \"'\\#"]*)
            fail "unsafe path: $path_to_validate"
            ;;
    esac
}

validate_address() {
    case "$1" in
        ""|*[" \"'\\#"]*) fail "invalid listen address: $1" ;;
    esac
    port=${1##*:}
    case "$port" in
        ""|*[!0-9]*) fail "address must end with a TCP port" ;;
    esac
    [ "$port" -ge 1 ] 2>/dev/null && [ "$port" -le 65535 ] 2>/dev/null ||
        fail "TCP port must be between 1 and 65535"
}

read_config_value() {
    key=$1
    awk -v key="$key" '
        index($0, key "=") == 1 { value = substr($0, length(key) + 2) }
        END { if (value != "") print value }
    ' "$CONFIG_FILE"
}

write_managed_config() {
    source_file="/dev/null"
    [ ! -f "$CONFIG_FILE" ] || source_file="$CONFIG_FILE"
    config_tmp=$(mktemp "$INSTALL_DIR/.wirely.env.XXXXXX")
    awk -v address="$ADDRESS" -v data_dir="$DATA_DIR" -v secure_cookie="$SECURE_COOKIE" -v queue_rate="$QUEUE_RATE" -v api_rate="$API_RATE_LIMIT" -v backup_interval="$BACKUP_INTERVAL" -v backup_retention="$BACKUP_RETENTION" '
        /^WIRELY_ADDRESS=/ {
            if (!address_seen) print "WIRELY_ADDRESS=" address
            address_seen = 1
            next
        }
        /^WIRELY_DATA_DIR=/ {
            if (!data_seen) print "WIRELY_DATA_DIR=" data_dir
            data_seen = 1
            next
        }
        /^WIRELY_SECURE_COOKIE=/ {
            if (!cookie_seen) print "WIRELY_SECURE_COOKIE=" secure_cookie
            cookie_seen = 1
            next
        }
        /^WIRELY_QUEUE_RATE=/ {
            if (!queue_seen) print
            queue_seen = 1
            next
        }
        /^WIRELY_API_RATE_LIMIT=/ { if (!api_rate_seen) print; api_rate_seen = 1; next }
        /^WIRELY_BACKUP_INTERVAL=/ { if (!backup_interval_seen) print; backup_interval_seen = 1; next }
        /^WIRELY_BACKUP_RETENTION=/ { if (!backup_retention_seen) print; backup_retention_seen = 1; next }
        { print }
        END {
            if (!address_seen) print "WIRELY_ADDRESS=" address
            if (!data_seen) print "WIRELY_DATA_DIR=" data_dir
            if (!cookie_seen) print "WIRELY_SECURE_COOKIE=" secure_cookie
            if (!queue_seen) print "WIRELY_QUEUE_RATE=" queue_rate
            if (!api_rate_seen) print "WIRELY_API_RATE_LIMIT=" api_rate
            if (!backup_interval_seen) print "WIRELY_BACKUP_INTERVAL=" backup_interval
            if (!backup_retention_seen) print "WIRELY_BACKUP_RETENTION=" backup_retention
        }
    ' "$source_file" > "$config_tmp"
    chown "$TARGET_USER:$TARGET_GROUP" "$config_tmp"
    chmod 0600 "$config_tmp"
    mv -f "$config_tmp" "$CONFIG_FILE"
}

ensure_download_tools() {
    missing_tools="false"
    for tool in curl tar sha256sum awk; do
        if ! command -v "$tool" >/dev/null 2>&1; then
            missing_tools="true"
        fi
    done
    if [ "$missing_tools" = "false" ]; then
        return
    fi
    command -v apt-get >/dev/null 2>&1 ||
        fail "curl, tar, awk, and sha256sum are required to download build tools"
    printf 'Installing download prerequisites...\n'
    apt-get update
    DEBIAN_FRONTEND=noninteractive apt-get install -y ca-certificates curl coreutils gawk tar xz-utils
}

host_architecture() {
    case "$(uname -m)" in
        x86_64|amd64) printf 'amd64\n' ;;
        aarch64|arm64) printf 'arm64\n' ;;
        *) fail "unsupported CPU architecture: $(uname -m)" ;;
    esac
}

go_is_compatible() {
    command -v go >/dev/null 2>&1 || return 1
    detected_go_version=$(go env GOVERSION 2>/dev/null | sed 's/^go//')
    version_at_least "$detected_go_version" "1.26.0"
}

node_is_compatible() {
    command -v node >/dev/null 2>&1 || return 1
    command -v npm >/dev/null 2>&1 || return 1
    detected_node_version=$(node --version 2>/dev/null | sed 's/^v//')
    version_at_least "$detected_node_version" "22.0.0"
}

bootstrap_go() {
    architecture=$1
    go_release=$(curl -fsSL 'https://go.dev/VERSION?m=text' | sed -n '1p')
    case "$go_release" in
        go[0-9]*.[0-9]*) ;;
        *) fail "could not determine the current Go release" ;;
    esac
    go_archive="$BUILD_TMP/${go_release}.linux-${architecture}.tar.gz"
    go_url="https://go.dev/dl/${go_release}.linux-${architecture}.tar.gz"
    printf 'Downloading %s for the build...\n' "$go_release"
    curl -fL "$go_url" -o "$go_archive"
    go_checksum=$(curl -fsSL "${go_url}.sha256")
    printf '%s  %s\n' "$go_checksum" "$go_archive" | sha256sum -c -
    mkdir -p "$BUILD_TMP/go"
    tar -xzf "$go_archive" -C "$BUILD_TMP/go" --strip-components=1
    PATH="$BUILD_TMP/go/bin:$PATH"
    export PATH
}

bootstrap_node() {
    architecture=$1
    node_architecture=$architecture
    [ "$architecture" != "amd64" ] || node_architecture="x64"
    sums_file="$BUILD_TMP/node-shasums.txt"
    curl -fsSL 'https://nodejs.org/dist/latest-v22.x/SHASUMS256.txt' -o "$sums_file"
    node_archive_name=$(awk -v suffix="-linux-${node_architecture}.tar.xz" '$2 ~ suffix "$" { print $2; exit }' "$sums_file")
    [ -n "$node_archive_name" ] || fail "could not determine the current Node.js 22 release"
    node_archive="$BUILD_TMP/$node_archive_name"
    printf 'Downloading %s for the build...\n' "$node_archive_name"
    curl -fL "https://nodejs.org/dist/latest-v22.x/$node_archive_name" -o "$node_archive"
    node_checksum=$(awk -v archive="$node_archive_name" '$2 == archive { print $1; exit }' "$sums_file")
    printf '%s  %s\n' "$node_checksum" "$node_archive" | sha256sum -c -
    mkdir -p "$BUILD_TMP/node"
    tar -xJf "$node_archive" -C "$BUILD_TMP/node" --strip-components=1
    PATH="$BUILD_TMP/node/bin:$PATH"
    export PATH
}

prepare_build_tools() {
    needs_go="false"
    needs_node="false"
    go_is_compatible || needs_go="true"
    node_is_compatible || needs_node="true"
    if [ "$needs_go" = "false" ] && [ "$needs_node" = "false" ]; then
        return
    fi

    ensure_download_tools
    BUILD_TMP=$(mktemp -d)
    trap cleanup EXIT
    trap 'cleanup; exit 130' HUP INT TERM
    architecture=$(host_architecture)

    if [ "$needs_go" = "true" ]; then
        bootstrap_go "$architecture"
        go_is_compatible || fail "downloaded Go toolchain is older than 1.26"
    fi
    if [ "$needs_node" = "true" ]; then
        bootstrap_node "$architecture"
        node_is_compatible || fail "downloaded Node.js toolchain is older than 22"
    fi
}

while [ "$#" -gt 0 ]; do
    case "$1" in
        --user)
            [ "$#" -ge 2 ] || fail "--user requires a value"
            TARGET_USER=$2
            shift 2
            ;;
        --install-dir)
            [ "$#" -ge 2 ] || fail "--install-dir requires a value"
            INSTALL_DIR=$2
            shift 2
            ;;
        --data-dir)
            [ "$#" -ge 2 ] || fail "--data-dir requires a value"
            DATA_DIR=$2
            DATA_DIR_SET="true"
            shift 2
            ;;
        --migrate-from)
            [ "$#" -ge 2 ] || fail "--migrate-from requires a value"
            MIGRATE_FROM=$2
            shift 2
            ;;
        --address)
            [ "$#" -ge 2 ] || fail "--address requires a value"
            ADDRESS=$2
            ADDRESS_SET="true"
            shift 2
            ;;
        --secure-cookie)
            SECURE_COOKIE="true"
            SECURE_COOKIE_SET="true"
            shift
            ;;
        --skip-build)
            BUILD="false"
            shift
            ;;
        --build-only)
            BUILD_ONLY="true"
            shift
            ;;
        --no-start)
            START_SERVICE="false"
            shift
            ;;
        --open-firewall)
            OPEN_FIREWALL="true"
            shift
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        *)
            fail "unknown option: $1"
            ;;
    esac
done

[ -f "$SCRIPT_DIR/go.mod" ] || fail "run install.sh from the Wirely source tree"
validate_address "$ADDRESS"

if [ "$BUILD_ONLY" = "false" ]; then
    [ "$(id -u)" -eq 0 ] || fail "run this installer as root (sudo ./install.sh)"

    if [ -z "$TARGET_USER" ]; then
        case "${SUDO_USER:-}" in
            ""|root) fail "could not detect the login user; pass --user USER" ;;
            *) TARGET_USER=$SUDO_USER ;;
        esac
    fi
    case "$TARGET_USER" in
        root|*[!a-zA-Z0-9_-]*|"") fail "invalid non-root Linux user: $TARGET_USER" ;;
    esac
    getent passwd "$TARGET_USER" >/dev/null 2>&1 || fail "Linux user does not exist: $TARGET_USER"
    TARGET_GROUP=$(id -gn "$TARGET_USER")
    TARGET_HOME=$(getent passwd "$TARGET_USER" | awk -F: '{ print $6 }')
    [ -n "$TARGET_HOME" ] || fail "could not determine home directory for $TARGET_USER"
    command -v readlink >/dev/null 2>&1 || fail "readlink is required"
    TARGET_HOME=$(readlink -m "$TARGET_HOME")

    [ -n "$INSTALL_DIR" ] || INSTALL_DIR="$TARGET_HOME/wirely"
    validate_path "$INSTALL_DIR"
    INSTALL_DIR=$(readlink -m "$INSTALL_DIR")
    case "$INSTALL_DIR/" in
        "$TARGET_HOME/"*) ;;
        *) fail "install directory must be inside $TARGET_HOME" ;;
    esac

    CONFIG_FILE="$INSTALL_DIR/wirely.env"
    if [ -f "$CONFIG_FILE" ]; then
        if [ "$ADDRESS_SET" = "false" ]; then
            configured_address=$(read_config_value WIRELY_ADDRESS)
            [ -z "$configured_address" ] || ADDRESS=$configured_address
        fi
        if [ "$DATA_DIR_SET" = "false" ]; then
            configured_data_dir=$(read_config_value WIRELY_DATA_DIR)
            [ -z "$configured_data_dir" ] || DATA_DIR=$configured_data_dir
        fi
        if [ "$SECURE_COOKIE_SET" = "false" ]; then
            configured_secure_cookie=$(read_config_value WIRELY_SECURE_COOKIE)
            [ -z "$configured_secure_cookie" ] || SECURE_COOKIE=$configured_secure_cookie
        fi
    fi

    [ -n "$DATA_DIR" ] || DATA_DIR="$INSTALL_DIR/data"
    validate_path "$DATA_DIR"
    DATA_DIR=$(readlink -m "$DATA_DIR")
    case "$DATA_DIR/" in
        "$INSTALL_DIR/"*) ;;
        *) fail "data directory must be inside $INSTALL_DIR" ;;
    esac
    validate_address "$ADDRESS"
    case "$SECURE_COOKIE" in
        true|false) ;;
        *) fail "WIRELY_SECURE_COOKIE must be true or false" ;;
    esac

    if [ -n "$MIGRATE_FROM" ]; then
        validate_path "$MIGRATE_FROM"
        MIGRATE_FROM=$(readlink -m "$MIGRATE_FROM")
        [ -d "$MIGRATE_FROM" ] || fail "migration source does not exist: $MIGRATE_FROM"
        [ "$MIGRATE_FROM" != "$DATA_DIR" ] || fail "migration source and destination must differ"
    fi

    INSTALL_BIN="$INSTALL_DIR/wirely"
    [ -f "$SCRIPT_DIR/deploy/wirely.service.template" ] || fail "systemd template is missing"
    command -v systemctl >/dev/null 2>&1 || fail "systemd is required"
    command -v install >/dev/null 2>&1 || fail "the install command is required"
fi

if [ "$BUILD" = "true" ]; then
    prepare_build_tools
    printf 'Building Wirely panel...\n'
    (cd "$SCRIPT_DIR/web" && npm ci && npm run build)

    printf 'Building Wirely API...\n'
    mkdir -p "$SCRIPT_DIR/bin"
    (cd "$SCRIPT_DIR" && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/wirely ./cmd/wirely)
fi

[ -x "$SCRIPT_DIR/bin/wirely" ] ||
    fail "bin/wirely was not found; remove --skip-build or provide a release binary"

if [ "$BUILD_ONLY" = "true" ]; then
    printf 'Wirely build created at %s/bin/wirely\n' "$SCRIPT_DIR"
    exit 0
fi

if [ -n "$MIGRATE_FROM" ]; then
    if [ -d "$DATA_DIR" ] && find "$DATA_DIR" -mindepth 1 -print -quit | grep -q .; then
        fail "migration destination is not empty: $DATA_DIR"
    fi
    if systemctl is-active --quiet "$SERVICE_NAME"; then
        printf 'Stopping %s for a consistent data migration...\n' "$SERVICE_NAME"
        systemctl stop "$SERVICE_NAME"
    fi
fi

install -d -m 0750 -o "$TARGET_USER" -g "$TARGET_GROUP" "$INSTALL_DIR"
install -d -m 0750 -o "$TARGET_USER" -g "$TARGET_GROUP" "$DATA_DIR"
if [ -n "$MIGRATE_FROM" ]; then
    cp -a "$MIGRATE_FROM/." "$DATA_DIR/"
    printf 'Data copied from %s (the source was preserved).\n' "$MIGRATE_FROM"
fi
chown -R "$TARGET_USER:$TARGET_GROUP" "$INSTALL_DIR"

write_managed_config
printf 'Configuration written to %s\n' "$CONFIG_FILE"

temporary_binary="$INSTALL_DIR/.wirely.new"
install -m 0755 -o "$TARGET_USER" -g "$TARGET_GROUP" "$SCRIPT_DIR/bin/wirely" "$temporary_binary"
mv -f "$temporary_binary" "$INSTALL_BIN"

sed \
    -e "s|@WIRELY_USER@|$TARGET_USER|g" \
    -e "s|@WIRELY_GROUP@|$TARGET_GROUP|g" \
    -e "s|@WIRELY_INSTALL_DIR@|$INSTALL_DIR|g" \
    -e "s|@WIRELY_DATA_DIR@|$DATA_DIR|g" \
    -e "s|@WIRELY_BIN@|$INSTALL_BIN|g" \
    -e "s|@WIRELY_ENV@|$CONFIG_FILE|g" \
    "$SCRIPT_DIR/deploy/wirely.service.template" > "$SYSTEMD_DIR/$SERVICE_NAME"
chmod 0644 "$SYSTEMD_DIR/$SERVICE_NAME"

systemctl daemon-reload
systemctl enable "$SERVICE_NAME" >/dev/null

if [ "$OPEN_FIREWALL" = "true" ]; then
    port=${ADDRESS##*:}
    if command -v ufw >/dev/null 2>&1 && ufw status 2>/dev/null | grep -q '^Status: active'; then
        ufw allow "$port/tcp"
    elif command -v iptables >/dev/null 2>&1; then
        if ! iptables -C INPUT -p tcp --dport "$port" -j ACCEPT 2>/dev/null; then
            iptables -I INPUT 1 -p tcp --dport "$port" -j ACCEPT
        fi
        printf 'Warning: the iptables rule may require a persistence package after reboot.\n'
    else
        printf 'Warning: no supported firewall manager was found; allow TCP port %s manually.\n' "$port"
    fi
fi

if [ "$START_SERVICE" = "true" ]; then
    systemctl restart "$SERVICE_NAME"
    if ! systemctl is-active --quiet "$SERVICE_NAME"; then
        journalctl -u "$SERVICE_NAME" -n 30 --no-pager >&2
        fail "service failed to start"
    fi
fi

printf '\nWirely API installed successfully.\n'
printf 'User: %s\n' "$TARGET_USER"
printf 'Directory: %s\n' "$INSTALL_DIR"
printf 'Binary: %s\n' "$INSTALL_BIN"
printf 'Configuration: %s\n' "$CONFIG_FILE"
printf 'Data: %s\n' "$DATA_DIR"
printf 'Listen address: %s\n' "$ADDRESS"

if [ "$START_SERVICE" = "true" ]; then
    printf '\nInitial credentials (shown only on first start):\n'
    journalctl -u "$SERVICE_NAME" --since "2 minutes ago" --no-pager 2>/dev/null |
        sed -n '/"initial_password"/p' || true
fi

printf '\nUseful commands:\n'
printf '  systemctl status %s\n' "$SERVICE_NAME"
printf '  journalctl -u %s -f\n' "$SERVICE_NAME"
