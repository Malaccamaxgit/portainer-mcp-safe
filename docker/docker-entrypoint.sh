#!/bin/sh
set -eu

portainer_server_url="${PORTAINER_SERVER_URL:-}"
portainer_api_token="${PORTAINER_API_TOKEN:-}"
portainer_read_only="${PORTAINER_READ_ONLY:-false}"
portainer_business_edition="${PORTAINER_BUSINESS_EDITION:-false}"
portainer_disable_version_check="${PORTAINER_DISABLE_VERSION_CHECK:-false}"
portainer_safe_mode="${PORTAINER_SAFE_MODE:-true}"
portainer_allow_unredacted_stack_content="${PORTAINER_ALLOW_UNREDACTED_STACK_CONTENT:-false}"
portainer_allow_sensitive_proxy_paths="${PORTAINER_ALLOW_SENSITIVE_PROXY_PATHS:-false}"
portainer_proxy_allowlist="${PORTAINER_PROXY_ALLOWLIST:-}"
portainer_extra_redaction_patterns="${PORTAINER_EXTRA_REDACTION_PATTERNS:-}"
portainer_tools_path="${PORTAINER_TOOLS_PATH:-/tmp/portainer-tools.yaml}"

is_template_placeholder() {
  case "${1:-}" in
    "{{"*"}}") return 0 ;;
    "<bool Value>"|"<int Value>"|"<float Value>"|"<string>"|"<value>") return 0 ;;
    *) return 1 ;;
  esac
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --server-url)
      portainer_server_url="$2"
      shift 2
      ;;
    --api-token)
      portainer_api_token="$2"
      shift 2
      ;;
    --read-only)
      if [ -n "$2" ]; then
        portainer_read_only="$2"
      fi
      shift 2
      ;;
    --business-edition)
      if [ -n "$2" ]; then
        portainer_business_edition="$2"
      fi
      shift 2
      ;;
    --disable-version-check)
      if [ -n "$2" ]; then
        portainer_disable_version_check="$2"
      fi
      shift 2
      ;;
    --safe-mode)
      if [ -n "$2" ]; then
        portainer_safe_mode="$2"
      fi
      shift 2
      ;;
    --allow-unredacted-stack-content)
      if [ -n "$2" ]; then
        portainer_allow_unredacted_stack_content="$2"
      fi
      shift 2
      ;;
    --allow-sensitive-proxy-paths)
      if [ -n "$2" ]; then
        portainer_allow_sensitive_proxy_paths="$2"
      fi
      shift 2
      ;;
    --proxy-allowlist)
      portainer_proxy_allowlist="$2"
      shift 2
      ;;
    --extra-redaction-patterns)
      portainer_extra_redaction_patterns="$2"
      shift 2
      ;;
    --tools-path)
      portainer_tools_path="$2"
      shift 2
      ;;
    *)
      echo "Unsupported argument: $1" >&2
      exit 1
      ;;
  esac
done

if is_template_placeholder "$portainer_read_only"; then
  portainer_read_only="false"
fi
if is_template_placeholder "$portainer_business_edition"; then
  portainer_business_edition="false"
fi
if is_template_placeholder "$portainer_disable_version_check"; then
  portainer_disable_version_check="false"
fi
if is_template_placeholder "$portainer_safe_mode"; then
  portainer_safe_mode="true"
fi
if is_template_placeholder "$portainer_allow_unredacted_stack_content"; then
  portainer_allow_unredacted_stack_content="false"
fi
if is_template_placeholder "$portainer_allow_sensitive_proxy_paths"; then
  portainer_allow_sensitive_proxy_paths="false"
fi
if is_template_placeholder "$portainer_proxy_allowlist"; then
  portainer_proxy_allowlist=""
fi
if is_template_placeholder "$portainer_extra_redaction_patterns"; then
  portainer_extra_redaction_patterns=""
fi

require_var() {
  name="$1"
  value="$2"

  if [ -z "$value" ]; then
    echo "$name must be set" >&2
    exit 1
  fi
}

is_true() {
  case "${1:-}" in
    1|true|TRUE|True|yes|YES|on|ON)
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

require_var PORTAINER_SERVER_URL "$portainer_server_url"
require_var PORTAINER_API_TOKEN "$portainer_api_token"

set -- \
  /usr/local/bin/portainer-mcp \
  -server "$portainer_server_url" \
  -token "$portainer_api_token" \
  -business-edition="$portainer_business_edition" \
  -safe-mode="$portainer_safe_mode" \
  -allow-unredacted-stack-content="$portainer_allow_unredacted_stack_content" \
  -allow-sensitive-proxy-paths="$portainer_allow_sensitive_proxy_paths" \
  -tools "$portainer_tools_path"

if is_true "$portainer_read_only"; then
  set -- "$@" -read-only
fi

if is_true "$portainer_disable_version_check"; then
  set -- "$@" -disable-version-check
fi

if [ -n "$portainer_proxy_allowlist" ]; then
  set -- "$@" -proxy-allowlist "$portainer_proxy_allowlist"
fi

if [ -n "$portainer_extra_redaction_patterns" ]; then
  set -- "$@" -extra-redaction-patterns "$portainer_extra_redaction_patterns"
fi

exec "$@"
