#!/bin/sh
set -eu

AYB_USER="${AYB_USER:-ayb}"
AYB_HOME="${HOME:-/home/ayb}"
AYB_RUNTIME_USER="$AYB_USER"
AYB_RUNTIME_UID="$(id -u "$AYB_USER")"
AYB_RUNTIME_GID="$(id -g "$AYB_USER")"

# The JWT signing key is generated and persisted here on first boot so no
# published key ever ships in a compose file or image. Mounting a volume at
# JWT_SECRET_DIR keeps the generated key across container recreation.
JWT_SECRET_DIR="$AYB_HOME/.ayb/secrets"
JWT_SECRET_FILE="$JWT_SECRET_DIR/jwt_secret"
JWT_INIT_SUBCOMMAND="__ayb_init_jwt_secret"

warn() {
  printf 'warning: %s\n' "$*" >&2
}

info() {
  printf '%s\n' "$*" >&2
}

# auth_is_enabled mirrors applyAuthCoreEnv in internal/config/config_env_auth.go,
# which treats only "true" and "1" as enabling auth. Auth defaults to disabled,
# so an unset value means no signing key is required.
auth_is_enabled() {
  case "${AYB_AUTH_ENABLED:-}" in
    true | 1) return 0 ;;
    *) return 1 ;;
  esac
}

# generate_jwt_secret_hex prints 32 random bytes as 64 lowercase hex characters.
# openssl is an explicit image dependency and is the primary source; the
# /dev/urandom fallback keeps generation working on a stripped-down base image.
generate_jwt_secret_hex() {
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -hex 32
    return 0
  fi
  if [ -r /dev/urandom ]; then
    head -c 32 /dev/urandom | od -v -An -tx1 | tr -d ' \n'
    return 0
  fi
  return 1
}

is_valid_jwt_secret_hex() {
  value="$1"
  case "$value" in
    *[!0-9a-f]*) return 1 ;;
  esac
  [ "${#value}" -eq 64 ]
}

# load_or_create_jwt_secret prints an established secret, or creates one, and
# always runs as the runtime user so the file lands with the right owner.
# Exit codes distinguish the failure modes the caller reports:
#   1 no generator available, 2 generated value invalid,
#   3 existing file unreadable, 4 existing file empty or malformed,
#   5 file mode could not be enforced to 0600,
#   6 directory creation or mode enforcement failed.
# An established key is never overwritten: an existing file is adopted, and the
# create uses noclobber so a racing boot loses rather than replacing the key.
load_or_create_jwt_secret() {
  file="$1"
  dir="$(dirname "$file")"
  mkdir -p "$dir" 2>/dev/null || return 6
  chmod 700 "$dir" 2>/dev/null || return 6

  if [ -e "$file" ]; then
    existing="$(cat "$file" 2>/dev/null)" || return 3
    is_valid_jwt_secret_hex "$existing" || return 4
    chmod 600 "$file" 2>/dev/null || return 5
    printf '%s' "$existing"
    return 0
  fi

  secret="$(generate_jwt_secret_hex)" || return 1
  is_valid_jwt_secret_hex "$secret" || return 2

  # umask 077 creates the file 0600 with no window at looser permissions.
  if ( umask 077; set -C; printf '%s' "$secret" > "$file" ) 2>/dev/null; then
    chmod 600 "$file" 2>/dev/null || return 5
    printf '%s' "$secret"
    return 0
  fi

  existing="$(cat "$file" 2>/dev/null)" || return 3
  is_valid_jwt_secret_hex "$existing" || return 4
  chmod 600 "$file" 2>/dev/null || return 5
  printf '%s' "$existing"
  return 0
}

report_jwt_secret_failure() {
  case "$1" in
    3) warn "JWT secret file $JWT_SECRET_FILE is not readable; refusing to start" ;;
    4) warn "JWT secret file $JWT_SECRET_FILE is empty or malformed; refusing to start (remove it to regenerate)" ;;
    5) warn "JWT secret file $JWT_SECRET_FILE could not be secured to mode 0600; refusing to start" ;;
    6) warn "JWT secret directory $JWT_SECRET_DIR could not be secured to mode 0700; refusing to start" ;;
    2) warn "generated JWT secret was not 64 hex characters; refusing to start" ;;
    *) warn "cannot generate a JWT secret (no openssl and no readable /dev/urandom); refusing to start" ;;
  esac
}

# configure_jwt_secret exports AYB_AUTH_JWT_SECRET before exec when auth is on
# and no secret was supplied. Mode "root" performs the file work through the
# resolved runtime user; mode "direct" is the already-non-root path.
configure_jwt_secret() {
  mode="$1"
  auth_is_enabled || return 0
  if [ -n "${AYB_AUTH_JWT_SECRET:-}" ]; then
    return 0
  fi

  if [ -e "$JWT_SECRET_FILE" ]; then
    secret_existed=1
  else
    secret_existed=0
  fi

  rc=0
  if [ "$mode" = "root" ]; then
    if ! ensure_writable_dir "$JWT_SECRET_DIR"; then
      warn "JWT secret directory $JWT_SECRET_DIR is not writable; refusing to start"
      exit 1
    fi
    secret="$(run_as_runtime "$0" "$JWT_INIT_SUBCOMMAND" "$JWT_SECRET_FILE")" || rc=$?
  else
    secret="$(load_or_create_jwt_secret "$JWT_SECRET_FILE")" || rc=$?
  fi

  if [ "$rc" -ne 0 ] || [ -z "${secret:-}" ]; then
    report_jwt_secret_failure "$rc"
    exit 1
  fi

  export AYB_AUTH_JWT_SECRET="$secret"
  if [ "$secret_existed" -eq 1 ]; then
    info "using persisted JWT signing secret from $JWT_SECRET_FILE"
  else
    info "generated a new JWT signing secret and persisted it to $JWT_SECRET_FILE"
  fi
}

# Re-entry point used by the root path to do the secret file work as the
# runtime user. Prints the secret on stdout for the parent to capture.
if [ "${1:-}" = "$JWT_INIT_SUBCOMMAND" ]; then
  set +e
  load_or_create_jwt_secret "$2"
  jwt_init_rc=$?
  set -e
  exit "$jwt_init_rc"
fi

lookup_group_name() {
  gid="$1"
  awk -F: -v gid="$gid" '$3 == gid { print $1; exit }' /etc/group
}

lookup_user_name() {
  uid="$1"
  awk -F: -v uid="$uid" '$3 == uid { print $1; exit }' /etc/passwd
}

ensure_runtime_group() {
  gid="$1"
  name="$(lookup_group_name "$gid")"
  if [ -n "$name" ]; then
    printf '%s\n' "$name"
    return 0
  fi
  name="aybhost"
  if grep -q "^${name}:" /etc/group; then
    name="${name}${gid}"
  fi
  printf '%s:x:%s:\n' "$name" "$gid" >> /etc/group
  printf '%s\n' "$name"
}

ensure_runtime_user() {
  uid="$1"
  gid="$2"
  name="$(lookup_user_name "$uid")"
  if [ -n "$name" ]; then
    printf '%s\n' "$name"
    return 0
  fi
  ensure_runtime_group "$gid" >/dev/null
  name="aybhost"
  if grep -q "^${name}:" /etc/passwd; then
    name="${name}${uid}"
  fi
  printf '%s:x:%s:%s:AYB runtime user:%s:/sbin/nologin\n' "$name" "$uid" "$gid" "$AYB_HOME" >> /etc/passwd
  printf '%s\n' "$name"
}

configure_runtime_user_from_owner() {
  owner_uid="$1"
  owner_gid="$2"
  if [ "$owner_uid" = "$AYB_RUNTIME_UID" ] && [ "$owner_gid" = "$AYB_RUNTIME_GID" ]; then
    return 0
  fi
  AYB_RUNTIME_USER="$(ensure_runtime_user "$owner_uid" "$owner_gid")"
  AYB_RUNTIME_UID="$owner_uid"
  AYB_RUNTIME_GID="$owner_gid"
}

configure_runtime_user_from_pgdata_dir() {
  dir="$1"
  if [ -z "$dir" ] || [ ! -e "$dir" ]; then
    return 0
  fi
  owner_uid="$(stat -c '%u' "$dir")"
  owner_gid="$(stat -c '%g' "$dir")"
  if [ "$owner_uid" = "0" ]; then
    return 0
  fi
  configure_runtime_user_from_owner "$owner_uid" "$owner_gid"
}

run_as_runtime() {
  su-exec "$AYB_RUNTIME_USER" "$@"
}

ensure_writable_dir() {
  dir="$1"
  if [ -z "$dir" ]; then
    return 0
  fi
  mkdir -p "$dir"
  if run_as_runtime test -w "$dir"; then
    return 0
  fi
  if ! chown -R "${AYB_RUNTIME_UID}:${AYB_RUNTIME_GID}" "$dir" 2>/dev/null; then
    warn "could not change ownership for $dir; continuing if write access already works"
  fi
  if run_as_runtime test -w "$dir"; then
    return 0
  fi
  warn "$dir is not writable for ${AYB_RUNTIME_USER}"
  return 1
}

ensure_writable_dirs() {
  for dir in "$@"; do
    ensure_writable_dir "$dir"
  done
}

if [ "$(id -u)" -eq 0 ]; then
  configure_runtime_user_from_pgdata_dir "${AYB_DATABASE_EMBEDDED_DATA_DIR:-}"
  ensure_writable_dirs \
    "$AYB_HOME" \
    "$AYB_HOME/.ayb" \
    "$AYB_HOME/.ayb/data" \
    "$AYB_HOME/.ayb/logs" \
    "$AYB_HOME/.ayb/run" \
    "${AYB_DATABASE_EMBEDDED_DATA_DIR:-}" \
    "${AYB_STORAGE_LOCAL_PATH:-}"

  configure_jwt_secret root

  export USER="$AYB_RUNTIME_USER"
  exec su-exec "$AYB_RUNTIME_USER" "$@"
fi

configure_jwt_secret direct

exec "$@"
