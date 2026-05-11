#!/usr/bin/env python3
"""HCP Cluster Lifecycle — create and delete GCP Hosted Control Plane clusters.

Usage:
    python hcp_lifecycle.py create <cluster-name>
    python hcp_lifecycle.py delete <cluster-name>
"""

import argparse
import base64
import json
import sys
import tempfile
import time
import os
from http.server import HTTPServer, BaseHTTPRequestHandler
from pathlib import Path

import requests
import yaml
from authlib.integrations.requests_client import OAuth2Session
from authlib.common.security import generate_token

from lib.crypto import generate_cluster_keypair
from lib.hypershift import (
    create_iam_gcp,
    create_infra_gcp,
    destroy_iam_gcp,
    destroy_infra_gcp,
    iam_config_to_wif_spec,
    validate_infra_id,
    HypershiftError,
)


def load_config(config_path: str = "config.yaml") -> dict:
    path = Path(config_path)
    if not path.exists():
        print(f"Error: {config_path} not found. Copy config.yaml.example to config.yaml and fill in values.")
        sys.exit(1)
    with open(path) as f:
        config = yaml.safe_load(f)
    required = [
        "keycloak_url", "keycloak_realm", "keycloak_client_id",
        "gcp_project", "workforce_pool", "workforce_provider",
        "broker_sa_email", "gateway_url", "gateway_audience",
        "project", "region",
    ]
    missing = [k for k in required if not config.get(k)]
    if missing:
        print(f"Error: missing required config keys: {', '.join(missing)}")
        sys.exit(1)
    return config


def keycloak_login(config: dict) -> tuple[str, str]:
    """Keycloak PKCE login. Opens browser, returns (id_token_jwt, user_email)."""
    base_url = config["keycloak_url"]
    realm = config["keycloak_realm"]
    client_id = config["keycloak_client_id"]

    authorize_url = f"{base_url}/realms/{realm}/protocol/openid-connect/auth"
    token_url = f"{base_url}/realms/{realm}/protocol/openid-connect/token"

    code_verifier = generate_token(48)

    session = OAuth2Session(
        client_id=client_id,
        code_challenge_method="S256",
    )

    uri, state = session.create_authorization_url(
        authorize_url,
        code_verifier=code_verifier,
        scope="openid email",
    )

    print("Opening browser for Keycloak login...")
    import webbrowser
    webbrowser.open(uri)

    print("Waiting for login callback on http://localhost:8888/callback ...")
    callback_url = _wait_for_callback()

    token_response = session.fetch_token(
        token_url,
        authorization_response=callback_url,
        code_verifier=code_verifier,
    )

    id_token = token_response.get("id_token")
    if not id_token:
        print("Error: no id_token in Keycloak response")
        sys.exit(1)

    payload = json.loads(
        base64.urlsafe_b64decode(id_token.split(".")[1] + "==")
    )
    email = payload.get("email")
    if not email:
        print("Error: no email claim in Keycloak token. Add an email mapper to the Keycloak client.")
        sys.exit(1)

    print(f"Logged in as: {email}")
    return id_token, email


def _wait_for_callback(port: int = 8888, timeout: int = 120) -> str:
    """Run a one-shot HTTP server to catch the OAuth callback."""
    callback_url = None

    class CallbackHandler(BaseHTTPRequestHandler):
        def do_GET(self):
            nonlocal callback_url
            callback_url = f"http://localhost:{port}{self.path}"
            self.send_response(200)
            self.send_header("Content-Type", "text/html")
            self.end_headers()
            self.wfile.write(b"<html><body><h2>Login successful. You can close this tab.</h2></body></html>")

        def log_message(self, format, *args):
            pass

    server = HTTPServer(("localhost", port), CallbackHandler)
    server.timeout = timeout
    server.handle_request()
    server.server_close()

    if not callback_url:
        print(f"Error: no callback received within {timeout} seconds")
        sys.exit(1)

    return callback_url


def sts_exchange(keycloak_jwt: str, config: dict) -> str:
    """Exchange Keycloak JWT for a Google Workforce access token via STS."""
    audience = (
        f"//iam.googleapis.com/locations/global/workforcePools/"
        f"{config['workforce_pool']}/providers/{config['workforce_provider']}"
    )
    resp = requests.post(
        "https://sts.googleapis.com/v1/token",
        data={
            "grant_type": "urn:ietf:params:oauth:grant-type:token-exchange",
            "audience": audience,
            "requested_token_type": "urn:ietf:params:oauth:token-type:access_token",
            "scope": "https://www.googleapis.com/auth/cloud-platform",
            "subject_token_type": "urn:ietf:params:oauth:token-type:jwt",
            "subject_token": keycloak_jwt,
        },
        headers={"Content-Type": "application/x-www-form-urlencoded"},
    )
    if resp.status_code >= 400:
        print(f"Error: STS exchange failed (HTTP {resp.status_code})")
        print(resp.text)
        sys.exit(1)

    token = resp.json().get("access_token")
    if not token:
        print("Error: no access_token in STS response")
        sys.exit(1)

    print("STS exchange succeeded.")
    return token


def generate_broker_id_token(workforce_token: str, config: dict) -> str:
    """Mint a Google-signed ID token for the broker service account."""
    url = (
        f"https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/"
        f"{config['broker_sa_email']}:generateIdToken"
    )
    resp = requests.post(
        url,
        headers={
            "Authorization": f"Bearer {workforce_token}",
            "Content-Type": "application/json",
            "x-goog-user-project": config["gcp_project"],
        },
        json={
            "audience": config["gateway_audience"],
            "includeEmail": True,
        },
    )
    if resp.status_code >= 400:
        print(f"Error: generateIdToken failed (HTTP {resp.status_code})")
        print(resp.text)
        sys.exit(1)

    token = resp.json().get("token")
    if not token:
        print("Error: no token in generateIdToken response")
        sys.exit(1)

    print("Broker ID token generated.")
    return token


def authenticate(config: dict) -> tuple[str, str]:
    """Full auth chain. Returns (broker_id_token, user_email)."""
    print("\n=== Authentication ===")
    keycloak_jwt, email = keycloak_login(config)
    workforce_token = sts_exchange(keycloak_jwt, config)
    broker_token = generate_broker_id_token(workforce_token, config)
    return broker_token, email
