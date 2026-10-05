# Store uploaded files with S3

Use object storage for user uploads, such as product photos. The application consumes S3-compatible bindings without naming a concrete storage product in its portable intent.

## Create an upload API scaffold

From a parent directory, with `baha` installed:

```bash
baha app new media-api --stack go --http --s3
cd media-api
baha plan
baha up -e dev
baha app env --format json
```

`up` requires a configured runtime Target. The Go scaffold prepares an AWS SDK S3 client and checks the bucket with `HeadBucket`. It does not generate an upload endpoint.

| Binding | Use |
| --- | --- |
| `S3_ENDPOINT`, `S3_BUCKET` | Provider endpoint and application bucket |
| `AWS_REGION` | SDK region |
| `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY` | Protected application credentials |
| `AWS_CA_BUNDLE` | Trust for the provider connection |

## Upload one photo from application code

In your handler, use the initialized `s3Client`, `bucket`, request context `ctx` and an `io.Reader` named `photo`:

```go
key := "products/mug-42/photo.jpg"
_, err := s3Client.PutObject(ctx, &s3.PutObjectInput{
    Bucket: &bucket,
    Key: &key,
    Body: photo,
})
```

Handle the SDK error before storing the object key in your database. Keep credentials in managed bindings; store object keys, not credential-bearing endpoint URLs, in business records. Verify storage readiness with `baha doctor` and include uploaded objects in your [recovery plan](backup-restore.md).
