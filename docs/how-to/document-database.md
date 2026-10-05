# Store product documents

Use `database.document/v1` for records with nested or varying fields, such as a product catalog with category-specific attributes. MongoDB is the reference provider; the application requests document storage rather than a product-specific deployment.

## Create the service

With `baha` installed, run from a parent directory:

```bash
baha app new catalog-documents --stack go --http --document-db
cd catalog-documents
baha plan
baha up -e dev
baha app env --format json
```

A configured runtime Target is required for `up`. The generated Go application includes the MongoDB driver and a TLS-configured `mongoClient`. `.env.example` declares `MONGODB_URL` and `MONGODB_CA_FILE`; normal environment output masks credential-bearing URLs.

## Add a document

In your request handler, use the initialized client and a request context `ctx`. Choose the application database provisioned for this deployment as `databaseName`, rather than another application's database. Import `go.mongodb.org/mongo-driver/v2/bson`:

```go
products := mongoClient.Database(databaseName).Collection("products")
_, err := products.InsertOne(ctx, bson.M{
    "sku": "mug-42",
    "name": "Coffee mug",
    "attributes": bson.M{"capacity_ml": 350, "color": "blue"},
})
```

This is business logic to add to the scaffold, not a generated catalog endpoint. Run `baha doctor` to check document-database readiness; include these durable records in your [recovery plan](backup-restore.md).
