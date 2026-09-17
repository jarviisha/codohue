#!/bin/sh
# This trusted init service holds database authority. Never mount its secrets in a consumer.
set -eu
/admin access bootstrap --username "${CODOHUE_BOOTSTRAP_USERNAME:-owner}" --secret-file /run/secrets/owner_password
/admin access token --name admin-proxy --secret-file /run/secrets/admin_proxy_token --permissions data:read,data:write --namespaces '*'
case "${CODOHUE_PROVISION_APPLICATION:-true}" in
  true) /admin access provision --name "${CODOHUE_APPLICATION_NAMESPACE:-app}" --secret-file /run/secrets/application_token --config-file /provision/namespace.json ;;
  false) ;; # Upgrade an existing installation without adopting or changing its namespaces.
  *) echo "CODOHUE_PROVISION_APPLICATION must be true or false" >&2; exit 1 ;;
esac
