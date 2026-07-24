#!/usr/bin/env bash

set -Eeuo pipefail

# ---------------------------------------------------------------------------
# EliteGate production deployment script
#
# Usage:
#   sudo /opt/elitegate/deploy.sh \
#     "<admin-image-uri>" \
#     "<gateway-image-uri>"
# ---------------------------------------------------------------------------

readonly ADMIN_IMAGE="${1:-}"
readonly GATEWAY_IMAGE="${2:-}"

readonly AWS_REGION="ap-south-1"
readonly AWS_ACCOUNT_ID="455540676403"
readonly ECR_REGISTRY="${AWS_ACCOUNT_ID}.dkr.ecr.${AWS_REGION}.amazonaws.com"

readonly DEPLOY_DIR="/opt/elitegate"
readonly ENV_FILE="${DEPLOY_DIR}/production.env"

readonly ADMIN_CONTAINER="elitegate-admin"
readonly PREVIOUS_ADMIN_CONTAINER="elitegate-admin-previous"

readonly DATABASE_SECRET="elitegate/production/database/postgres"
readonly REDIS_SECRET="elitegate/production/redis/cache"
readonly JWT_SECRET_NAME="elitegate/production/auth/jwt"
readonly OAUTH_SECRET="elitegate/production/auth/oauth"
readonly SMTP_SECRET="elitegate/production/email/smtp"

log() {
  printf '[%s] %s\n' "$(date -u '+%Y-%m-%dT%H:%M:%SZ')" "$*"
}

fail() {
  log "ERROR: $*"
  exit 1
}

require_command() {
  command -v "$1" >/dev/null 2>&1 ||
    fail "Required command is missing: $1"
}

get_parameter() {
  local parameter_name="$1"

  aws ssm get-parameter \
    --name "$parameter_name" \
    --region "$AWS_REGION" \
    --query 'Parameter.Value' \
    --output text
}

get_secret() {
  local secret_name="$1"

  aws secretsmanager get-secret-value \
    --secret-id "$secret_name" \
    --region "$AWS_REGION" \
    --query 'SecretString' \
    --output text
}

validate_inputs() {
  [[ -n "$ADMIN_IMAGE" ]] ||
    fail "Admin image URI was not provided."

  [[ -n "$GATEWAY_IMAGE" ]] ||
    fail "Gateway image URI was not provided."

  [[ "$ADMIN_IMAGE" == "${ECR_REGISTRY}/elitegate-admin:"* ]] ||
    fail "Unexpected Admin image URI: $ADMIN_IMAGE"

  [[ "$GATEWAY_IMAGE" == "${ECR_REGISTRY}/elitegate-gateway:"* ]] ||
    fail "Unexpected Gateway image URI: $GATEWAY_IMAGE"
}

prepare_directory() {
  mkdir -p "${DEPLOY_DIR}/releases"
  chmod 750 "$DEPLOY_DIR"
}

login_to_ecr() {
  log "Logging in to Amazon ECR..."

  aws ecr get-login-password --region "$AWS_REGION" |
    docker login \
      --username AWS \
      --password-stdin "$ECR_REGISTRY"
}

pull_images() {
  log "Pulling Admin image: $ADMIN_IMAGE"
  docker pull "$ADMIN_IMAGE"

  log "Pulling Gateway image: $GATEWAY_IMAGE"
  docker pull "$GATEWAY_IMAGE"
}

build_environment_file() {
  log "Loading production configuration from AWS..."

  local database_json
  local redis_json
  local jwt_json
  local oauth_json
  local smtp_json

  database_json="$(get_secret "$DATABASE_SECRET")"
  redis_json="$(get_secret "$REDIS_SECRET")"
  jwt_json="$(get_secret "$JWT_SECRET_NAME")"
  oauth_json="$(get_secret "$OAUTH_SECRET")"
  smtp_json="$(get_secret "$SMTP_SECRET")"

  local app_environment
  local admin_port
  local gateway_port

  app_environment="$(get_parameter "/elitegate/production/app/environment")"
  admin_port="$(get_parameter "/elitegate/production/admin/port")"
  gateway_port="$(get_parameter "/elitegate/production/gateway/port")"

  local db_username
  local db_password
  local db_host
  local db_port
  local db_name

  db_username="$(jq -r '.username' <<<"$database_json")"
  db_password="$(jq -r '.password' <<<"$database_json")"
  db_host="$(jq -r '.host' <<<"$database_json")"
  db_port="$(jq -r '.port' <<<"$database_json")"
  db_name="$(jq -r '.dbname' <<<"$database_json")"

  local db_username_encoded
  local db_password_encoded

  db_username_encoded="$(
    jq -nr --arg value "$db_username" '$value | @uri'
  )"

  db_password_encoded="$(
    jq -nr --arg value "$db_password" '$value | @uri'
  )"

  local redis_token
  local redis_endpoint
  local redis_port
  local redis_token_encoded

  redis_token="$(jq -r '.auth_token' <<<"$redis_json")"
  redis_endpoint="$(jq -r '.primary_endpoint' <<<"$redis_json")"
  redis_port="$(jq -r '.port' <<<"$redis_json")"
  redis_token_encoded="$(
  jq -nr --arg value "$redis_token" '$value | @uri'
)"

  local jwt_secret
  jwt_secret="$(jq -r '.jwt_secret' <<<"$jwt_json")"

  local google_client_id
  local google_client_secret
  local oauth_state_secret
  local google_redirect_url

  google_client_id="$(jq -r '.google_client_id' <<<"$oauth_json")"
  google_client_secret="$(jq -r '.google_client_secret' <<<"$oauth_json")"
  oauth_state_secret="$(jq -r '.oauth_state_secret' <<<"$oauth_json")"
  google_redirect_url="$(jq -r '.google_redirect_url' <<<"$oauth_json")"

  local smtp_enabled
  local smtp_host
  local smtp_port
  local smtp_username
  local smtp_password
  local smtp_from_email
  local smtp_from_name
  local smtp_tls_mode
  local password_reset_url

  smtp_enabled="$(jq -r '.smtp_enabled' <<<"$smtp_json")"
  smtp_host="$(jq -r '.smtp_host' <<<"$smtp_json")"
  smtp_port="$(jq -r '.smtp_port' <<<"$smtp_json")"
  smtp_username="$(jq -r '.smtp_username' <<<"$smtp_json")"
  smtp_password="$(jq -r '.smtp_password' <<<"$smtp_json")"
  smtp_from_email="$(jq -r '.smtp_from_email' <<<"$smtp_json")"
  smtp_from_name="$(jq -r '.smtp_from_name' <<<"$smtp_json")"
  smtp_tls_mode="$(jq -r '.smtp_tls_mode' <<<"$smtp_json")"
  password_reset_url="$(jq -r '.password_reset_url' <<<"$smtp_json")"

  local temporary_env
  temporary_env="$(mktemp "${DEPLOY_DIR}/production.env.XXXXXX")"

  cat >"$temporary_env" <<EOF
APP_ENV=${app_environment}

ADMIN_PORT=${admin_port}
GATEWAY_PORT=${gateway_port}

POSTGRES_DSN=postgres://${db_username_encoded}:${db_password_encoded}@${db_host}:${db_port}/${db_name}?sslmode=require
POSTGRES_GATEWAY_DSN=postgres://${db_username_encoded}:${db_password_encoded}@${db_host}:${db_port}/${db_name}?sslmode=require

REDIS_ADDR=rediss://:${redis_token_encoded}@${redis_endpoint}:${redis_port}

JWT_SECRET=${jwt_secret}

GOOGLE_CLIENT_ID=${google_client_id}
GOOGLE_CLIENT_SECRET=${google_client_secret}
GOOGLE_REDIRECT_URL=${google_redirect_url}
OAUTH_STATE_SECRET=${oauth_state_secret}

SMTP_ENABLED=${smtp_enabled}
SMTP_HOST=${smtp_host}
SMTP_PORT=${smtp_port}
SMTP_USERNAME=${smtp_username}
SMTP_PASSWORD=${smtp_password}
SMTP_FROM_EMAIL=${smtp_from_email}
SMTP_FROM_NAME=${smtp_from_name}
SMTP_TLS_MODE=${smtp_tls_mode}
PASSWORD_RESET_URL=${password_reset_url}
GATEWAY_IMAGE_NAME=${GATEWAY_IMAGE}
EOF

  chown root:root "$temporary_env"
  chmod 600 "$temporary_env"

  mv "$temporary_env" "$ENV_FILE"

  log "Production environment file generated securely."
}

backup_current_container() {
  docker rm -f "$PREVIOUS_ADMIN_CONTAINER" >/dev/null 2>&1 || true

  if docker container inspect "$ADMIN_CONTAINER" >/dev/null 2>&1; then
    log "Preserving current Admin container for rollback..."

    docker stop "$ADMIN_CONTAINER"
    docker rename "$ADMIN_CONTAINER" "$PREVIOUS_ADMIN_CONTAINER"
  else
    log "No existing Admin container found. This appears to be the first deployment."
  fi
}

start_new_admin() {
  log "Starting the new Admin container..."

  local docker_socket_gid

  [[ -S /var/run/docker.sock ]] ||
    fail "Docker socket was not found at /var/run/docker.sock."

  docker_socket_gid="$(stat -c '%g' /var/run/docker.sock)"

  [[ "$docker_socket_gid" =~ ^[0-9]+$ ]] ||
    fail "Could not determine Docker socket group ID."

  log "Granting Admin container access to Docker socket group ID ${docker_socket_gid}."

  docker run -d \
    --name "$ADMIN_CONTAINER" \
    --restart unless-stopped \
    --env-file "$ENV_FILE" \
    --network elitegate_net \
    --group-add "$docker_socket_gid" \
    -p 9090:9090 \
    -v /var/run/docker.sock:/var/run/docker.sock \
    "$ADMIN_IMAGE"
}

wait_for_health() {
  log "Waiting for Admin health check..."

  local attempts=30
  local delay_seconds=5

  for ((attempt = 1; attempt <= attempts; attempt++)); do
    if curl --fail --silent --show-error \
      "http://127.0.0.1:9090/healthz" >/dev/null; then
      log "Health check passed."
      return 0
    fi

    log "Health check attempt ${attempt}/${attempts} failed."
    sleep "$delay_seconds"
  done

  return 1
}

rollback() {
  log "Deployment failed. Starting rollback..."

  docker logs "$ADMIN_CONTAINER" --tail 100 || true
  docker rm -f "$ADMIN_CONTAINER" >/dev/null 2>&1 || true

  if docker container inspect "$PREVIOUS_ADMIN_CONTAINER" >/dev/null 2>&1; then
    docker rename "$PREVIOUS_ADMIN_CONTAINER" "$ADMIN_CONTAINER"
    docker start "$ADMIN_CONTAINER"

    log "Previous Admin container restored."
  else
    log "No previous Admin container was available for rollback."
  fi

  exit 1
}

finish_deployment() {
  docker rm -f "$PREVIOUS_ADMIN_CONTAINER" >/dev/null 2>&1 || true

  docker image prune -f >/dev/null 2>&1 || true

  log "Deployment completed successfully."
  log "Admin image: $ADMIN_IMAGE"
  log "Gateway image for new tenant gateways: $GATEWAY_IMAGE"
}

main() {
  require_command aws
  require_command docker
  require_command jq
  require_command curl

  validate_inputs
  prepare_directory
  login_to_ecr
  pull_images
  build_environment_file
  backup_current_container
  start_new_admin

  if ! wait_for_health; then
    rollback
  fi

  finish_deployment
}

main "$@"