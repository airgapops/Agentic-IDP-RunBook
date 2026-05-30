#!/bin/bash
set -e
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml

THUNDER_POD=$(kubectl get pod -n thunder -l app.kubernetes.io/name=thunder -o jsonpath='{.items[0].metadata.name}')
echo "Using Thunder pod: $THUNDER_POD"

# Thunder setup mode runs the server with --setup flag which disables auth on management APIs
# We need to run the setup commands from within the pod using the setup binary

echo "==> Creating setup script inside pod..."
kubectl exec -n thunder "$THUNDER_POD" -- sh -c '
THUNDER_URL="http://localhost:8090"

echo "Step 1: Create org unit..."
curl -s --location "${THUNDER_URL}/setup/organization-units" \
  --header "Content-Type: application/json" \
  --data "{
    \"name\": \"Default\",
    \"handle\": \"default\",
    \"description\": \"Default organizational unit\"
  }" --max-time 30 || true

echo ""
echo "Step 2: Get org unit ID..."
ORG_UNIT_ID=$(curl -s --max-time 10 "${THUNDER_URL}/setup/organization-units/tree/default" | grep -o "\"id\":\"[^\"]*\"" | head -1 | cut -d"\"" -f4)
if [ -z "$ORG_UNIT_ID" ]; then
  # Try without /setup prefix
  ORG_UNIT_ID=$(curl -s --max-time 10 "${THUNDER_URL}/organization-units/tree/default" 2>/dev/null | grep -o "\"id\":\"[^\"]*\"" | head -1 | cut -d"\"" -f4)
fi
echo "ORG_UNIT_ID=$ORG_UNIT_ID"

echo "Step 3: Create user schema..."
curl -s --location "${THUNDER_URL}/setup/user-schemas" \
  --header "Content-Type: application/json" \
  --data "{
    \"name\": \"openchoreo-user\",
    \"ouId\": \"$ORG_UNIT_ID\",
    \"allowSelfRegistration\": true,
    \"systemAttributes\": { \"display\": \"username\" },
    \"schema\": {
      \"username\": { \"type\": \"string\", \"required\": true, \"displayName\": \"Username\" },
      \"password\": { \"type\": \"string\", \"required\": true, \"credential\": true, \"displayName\": \"Password\" },
      \"given_name\": { \"type\": \"string\", \"required\": true, \"displayName\": \"First Name\" },
      \"family_name\": { \"type\": \"string\", \"required\": true, \"displayName\": \"Last Name\" }
    }
  }" --max-time 30 || true

echo ""
echo "Step 4: Get user schema ID..."
USER_SCHEMA_ID=$(curl -s --max-time 10 "${THUNDER_URL}/setup/user-schemas" 2>/dev/null | grep -o "\"id\":\"[^\"]*\"" | head -1 | cut -d"\"" -f4)
if [ -z "$USER_SCHEMA_ID" ]; then
  USER_SCHEMA_ID=$(curl -s --max-time 10 "${THUNDER_URL}/user-schemas" 2>/dev/null | grep -o "\"id\":\"[^\"]*\"" | head -1 | cut -d"\"" -f4)
fi
echo "USER_SCHEMA_ID=$USER_SCHEMA_ID"

echo "Step 5: Create admin user..."
curl -s --location "${THUNDER_URL}/setup/user-schemas/${USER_SCHEMA_ID}/users" \
  --header "Content-Type: application/json" \
  --data "{
    \"username\": \"admin@openchoreo.dev\",
    \"password\": \"Admin@123\",
    \"given_name\": \"Admin\",
    \"family_name\": \"User\"
  }" --max-time 30 || true

echo ""
echo "Step 6: Create OAuth app for Backstage..."
curl -s --location "${THUNDER_URL}/setup/applications" \
  --header "Content-Type: application/json" \
  --data "{
    \"name\": \"openchoreo-backstage\",
    \"ouId\": \"$ORG_UNIT_ID\",
    \"clientId\": \"openchoreo-backstage-client\",
    \"clientSecret\": \"backstageclientsecret456\",
    \"redirectUris\": [
      \"http://choreo.zerops.internal:8080/api/auth/thunder/handler/frame\",
      \"http://choreo.zerops.internal:8080/api/auth/openchoreo-auth/handler/frame\"
    ],
    \"grantTypes\": [\"authorization_code\", \"refresh_token\", \"client_credentials\"],
    \"responseTypes\": [\"code\"],
    \"tokenEndpointAuthMethod\": \"client_secret_post\"
  }" --max-time 30 || true

echo ""
echo "DONE"
'
