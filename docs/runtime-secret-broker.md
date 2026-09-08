# Runtime secret broker

Applications that use BaseHarbor dynamic managed secrets receive a per-application broker automatically.

The application does not configure broker topology or PKI. BaseHarbor injects:

```text
BASEHARBOR_RUNTIME_API_URL=https://baseharbor-secrets:8443
BASEHARBOR_RUNTIME_TOKEN_FILE=/run/secrets/baseharbor-runtime-token
BASEHARBOR_RUNTIME_CA_FILE=/run/secrets/baseharbor-runtime-ca
BASEHARBOR_RUNTIME_CLIENT_CERT_FILE=/run/secrets/baseharbor-runtime-client-cert
BASEHARBOR_RUNTIME_CLIENT_KEY_FILE=/run/secrets/baseharbor-runtime-client-key
```

The application uses ordinary HTTPS with the supplied client certificate, client key and CA file. Requests also carry the app-scoped runtime bearer token. No `baha login`, human OIDC token, Docker socket, OpenBao manager credential or mandatory BaseHarbor SDK is required.

The broker accepts only the application identity encoded as:

```text
spiffe://baseharbor/apps/<app>/<environment>
```

and its OpenBao identity is limited to that application's managed secret namespace.
