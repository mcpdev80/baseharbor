# Supply a payment API secret

Declare the secret name in application intent and supply its value through a protected input path. Never commit the value in the manifest or `.env.example`.

## Create the application

From a parent directory, with `baha` installed:

```bash
baha app new billing-api --stack go --http --secrets --require-secret PAYMENT_API_KEY
cd billing-api
baha up -e dev
```

A configured runtime Target is required. The first interactive `up` asks for the missing required secret with terminal echo disabled. The generated application expects `PAYMENT_API_KEY`; `.env.example` contains its name with an empty value.

## Replace the value later

After the application's secret service is ready, enter a replacement interactively:

```bash
baha app secret set PAYMENT_API_KEY
baha app secret list
```

For automation, prepare an owner-only file outside the repository through your secret manager, then use it without putting the secret itself in command arguments:

```bash
baha --no-input app secret set PAYMENT_API_KEY --file "$HOME/.config/billing/payment-api-key"
```

The path must exist and contain the intended value. Normal list/status/evidence output must not reveal it. Reconcile the application with `baha up`, then verify readiness with `baha doctor`.

For application code that needs dynamic secret access rather than startup bindings, see [dynamic application secrets](dynamic-application-secrets.md).
